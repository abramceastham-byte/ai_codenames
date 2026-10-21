package botproto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/bcspragu/Codenames/codenames"
	"github.com/bcspragu/Codenames/game"
)

// maxStderrBytes caps how much of a bot's stderr is kept in the log.
const maxStderrBytes = 4 * 1024

// Player is one student's pair of bot executables. Each command is an argv,
// e.g. []string{"python3", "bots/alice/spymaster.py"}.
type Player struct {
	Name      string   `json:"name"`
	Spymaster []string `json:"spymaster"`
	Operative []string `json:"operative"`
}

// Options tunes a match. The zero value uses the defaults.
type Options struct {
	// MoveTimeout is how long a bot has per move. Defaults to DefaultMoveTimeout.
	MoveTimeout time.Duration
	// MaxTurns ends the game after this many turns. Defaults to MaxTurns.
	MaxTurns int
	// Dir is the working directory bots are started in.
	Dir string
}

// Event is a logged bot failure.
type Event struct {
	Turn   int    `json:"turn"`
	Team   Team   `json:"team"`
	Role   Role   `json:"role"`
	Bot    string `json:"bot"`
	Error  string `json:"error"`
	Stderr string `json:"stderr,omitempty"`
}

// Result is the outcome of one match.
type Result struct {
	// Winner is empty for a draw.
	Winner Team `json:"winner"`
	// Reason explains how the game ended.
	Reason string `json:"reason"`
	// Tiebreak is the rule that decided a game that hit the turn limit
	// (TiebreakForfeits or TiebreakCardsLeft), and empty otherwise.
	Tiebreak string  `json:"tiebreak,omitempty"`
	History  []Turn  `json:"history"`
	Events   []Event `json:"events"`
	// Board is the final board, with every card's type.
	Board []Card `json:"board"`
}

// RunMatch plays one game between two players on the given board. It only
// returns an error if the engine itself fails; bot failures are forfeits
// recorded in Result.Events.
func RunMatch(ctx context.Context, red, blue Player, board *codenames.Board, starter codenames.Team, opts Options) (*Result, error) {
	if opts.MoveTimeout == 0 {
		opts.MoveTimeout = DefaultMoveTimeout
	}
	if opts.MaxTurns == 0 {
		opts.MaxTurns = MaxTurns
	}
	m := &match{
		players: map[Team]Player{TeamRed: red, TeamBlue: blue},
		opts:    opts,
		state: &codenames.GameState{
			StartingTeam: starter,
			ActiveTeam:   starter,
			ActiveRole:   codenames.SpymasterRole,
			Board:        board.Clone(),
			Clues:        []codenames.SpymasterClue{},
		},
		res: &Result{History: []Turn{}, Events: []Event{}},
	}
	m.g = game.NewForMove(m.state)

	for len(m.res.History) < opts.MaxTurns {
		over, err := m.playTurn(ctx)
		if err != nil {
			return nil, err
		}
		if over {
			_, winner := codenames.GameOver(m.state)
			m.res.Winner = fromTeam(winner)
			if m.revealed(codenames.Assassin) {
				m.res.Reason = "assassin revealed"
			} else {
				m.res.Reason = "all agents found"
			}
			m.res.Board = CardsFromBoard(m.state.Board, true)
			return m.res, nil
		}
	}

	// Turn limit: the team with more forfeits loses, then fewer cards left wins.
	redForfeits, blueForfeits := m.forfeits(TeamRed), m.forfeits(TeamBlue)
	if redForfeits != blueForfeits {
		m.res.Tiebreak = TiebreakForfeits
		m.res.Winner = TeamRed
		if redForfeits > blueForfeits {
			m.res.Winner = TeamBlue
		}
		m.res.Reason = fmt.Sprintf("turn limit reached, decided by forfeits (red %d, blue %d)", redForfeits, blueForfeits)
	} else {
		redLeft, blueLeft := m.left(codenames.RedAgent), m.left(codenames.BlueAgent)
		m.res.Tiebreak = TiebreakCardsLeft
		switch {
		case redLeft < blueLeft:
			m.res.Winner = TeamRed
		case blueLeft < redLeft:
			m.res.Winner = TeamBlue
		}
		m.res.Reason = fmt.Sprintf("turn limit reached, decided by cards left (red %d left, blue %d left)", redLeft, blueLeft)
	}
	m.res.Board = CardsFromBoard(m.state.Board, true)
	return m.res, nil
}

const (
	// TiebreakForfeits: the team with more forfeits lost.
	TiebreakForfeits = "forfeits"
	// TiebreakCardsLeft: forfeits were equal, so the team with fewer of its
	// own cards left won (or it was a draw).
	TiebreakCardsLeft = "cards_left"
)

type match struct {
	players map[Team]Player
	opts    Options
	state   *codenames.GameState
	g       *game.Game
	res     *Result
}

// playTurn runs one clue and its guesses, and reports whether the game ended.
func (m *match) playTurn(ctx context.Context) (bool, error) {
	team := fromTeam(m.state.ActiveTeam)
	player := m.players[team]
	turn := Turn{Team: team, Guesses: []Guess{}}
	defer func() { m.res.History = append(m.res.History, turn) }()

	// Spymaster
	var sr SpymasterResponse
	err := m.ask(ctx, player.Spymaster, m.request(RoleSpymaster, nil), &sr)
	if err == nil {
		err = ValidateClue(sr, CardsFromBoard(m.state.Board, true))
	}
	if err != nil {
		m.logFailure(team, RoleSpymaster, player.Name, err)
		turn.End = EndSpymasterForfeit
		return false, m.forfeit()
	}
	clue := &Clue{Word: strings.ToLower(strings.TrimSpace(sr.Clue)), Count: sr.Count}
	turn.Clue = clue
	if _, _, err := m.g.Move(&game.Move{
		Action:   game.ActionGiveClue,
		Team:     m.state.ActiveTeam,
		GiveClue: &codenames.Clue{Word: clue.Word, Count: clue.Count},
	}); err != nil {
		return false, fmt.Errorf("giving clue %q: %w", clue.Word, err)
	}

	// Operative
	var or OperativeResponse
	if err := m.ask(ctx, player.Operative, m.request(RoleOperative, clue), &or); err != nil {
		m.logFailure(team, RoleOperative, player.Name, err)
		turn.End = EndOperativeForfeit
		return false, m.forfeit()
	}
	for _, word := range or.Guesses {
		card, err := m.findGuess(word)
		if err != nil {
			m.logFailure(team, RoleOperative, player.Name, err)
			turn.End = EndOperativeForfeit
			return false, m.forfeit()
		}
		_, status, err := m.g.Move(&game.Move{
			Action: game.ActionGuess,
			Team:   m.state.ActiveTeam,
			Guess:  card.Codename,
		})
		if err != nil {
			return false, fmt.Errorf("guessing %q: %w", card.Codename, err)
		}
		turn.Guesses = append(turn.Guesses, Guess{Word: card.Codename, Result: fromAgent(card.Agent)})

		switch {
		case status == codenames.Finished:
			turn.End = EndGameOver
			return true, nil
		case m.state.ActiveRole == codenames.SpymasterRole && card.Agent == ownAgent(team):
			turn.End = EndOutOfGuesses
			return false, nil
		case m.state.ActiveRole == codenames.SpymasterRole:
			turn.End = EndWrongCard
			return false, nil
		}
	}

	// The guess list ran out with guesses to spare: pass.
	turn.End = EndStopped
	if _, _, err := m.g.Move(&game.Move{Action: game.ActionGuess, Team: m.state.ActiveTeam}); err != nil {
		return false, fmt.Errorf("passing: %w", err)
	}
	return false, nil
}

func (m *match) forfeit() error {
	if _, _, err := m.g.Move(&game.Move{Action: game.ActionForfeit, Team: m.state.ActiveTeam}); err != nil {
		return fmt.Errorf("forfeiting: %w", err)
	}
	return nil
}

// findGuess returns the unrevealed card matching word, or why it's illegal.
func (m *match) findGuess(word string) (codenames.Card, error) {
	for _, c := range m.state.Board.Cards {
		if !strings.EqualFold(c.Codename, strings.TrimSpace(word)) {
			continue
		}
		if c.Revealed {
			return codenames.Card{}, fmt.Errorf("guess %q is already revealed", word)
		}
		return c, nil
	}
	return codenames.Card{}, fmt.Errorf("guess %q is not on the board", word)
}

func (m *match) request(role Role, clue *Clue) *Request {
	req := &Request{
		ProtocolVersion: ProtocolVersion,
		Role:            role,
		Team:            fromTeam(m.state.ActiveTeam),
		Board:           CardsFromBoard(m.state.Board, role == RoleSpymaster),
		History:         m.res.History,
	}
	if clue != nil {
		req.Clue = clue
		req.GuessesAllowed = clue.Count + 1
	}
	return req
}

// botError is a bot failure, carrying whatever the bot wrote to stderr.
type botError struct {
	err    error
	stderr string
}

func (e *botError) Error() string { return e.err.Error() }

// ask runs one bot process, sends it req, and decodes its response into out.
func (m *match) ask(ctx context.Context, argv []string, req *Request, out any) error {
	if len(argv) == 0 {
		return errors.New("no command configured")
	}
	in, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, m.opts.MoveTimeout)
	defer cancel()

	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = MaxResponseBytes, maxStderrBytes
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = m.opts.Dir
	cmd.Stdin = bytes.NewReader(in)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Don't wait forever on pipes held open by a bot's child processes.
	cmd.WaitDelay = time.Second

	runErr := cmd.Run()
	fail := func(err error) error { return &botError{err: err, stderr: stderr.String()} }
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return fail(fmt.Errorf("timed out after %s", m.opts.MoveTimeout))
	case runErr != nil:
		return fail(fmt.Errorf("bot failed: %w", runErr))
	case stdout.overflow:
		return fail(fmt.Errorf("response is larger than %d bytes", MaxResponseBytes))
	}

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := dec.Decode(out); err != nil {
		return fail(fmt.Errorf("invalid JSON response: %w", err))
	}
	if _, err := dec.Token(); err != io.EOF {
		return fail(errors.New("response has extra output after the JSON object"))
	}
	return nil
}

func (m *match) logFailure(team Team, role Role, bot string, err error) {
	ev := Event{Turn: len(m.res.History) + 1, Team: team, Role: role, Bot: bot, Error: err.Error()}
	var be *botError
	if errors.As(err, &be) {
		ev.Stderr = be.stderr
	}
	m.res.Events = append(m.res.Events, ev)
}

func (m *match) revealed(agent codenames.Agent) bool {
	for _, c := range m.state.Board.Cards {
		if c.Revealed && c.Agent == agent {
			return true
		}
	}
	return false
}

func (m *match) forfeits(team Team) int {
	n := 0
	for _, ev := range m.res.Events {
		if ev.Team == team {
			n++
		}
	}
	return n
}

func (m *match) left(agent codenames.Agent) int {
	n := 0
	for _, c := range m.state.Board.Cards {
		if !c.Revealed && c.Agent == agent {
			n++
		}
	}
	return n
}

// limitedBuffer keeps the first max bytes written to it and notes overflow.
type limitedBuffer struct {
	bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); len(p) > room {
		b.overflow = true
		if room > 0 {
			b.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

// CardsFromBoard converts an engine board to protocol cards. With showTypes
// false, unrevealed cards have no Type, as operatives see them.
func CardsFromBoard(b *codenames.Board, showTypes bool) []Card {
	out := make([]Card, len(b.Cards))
	for i, c := range b.Cards {
		out[i] = Card{Word: c.Codename, Revealed: c.Revealed}
		if showTypes || c.Revealed {
			out[i].Type = fromAgent(c.Agent)
		}
		if c.Revealed {
			out[i].RevealedBy = fromTeam(c.RevealedBy)
		}
	}
	return out
}

func fromTeam(t codenames.Team) Team {
	switch t {
	case codenames.RedTeam:
		return TeamRed
	case codenames.BlueTeam:
		return TeamBlue
	}
	return ""
}

func fromAgent(a codenames.Agent) CardType {
	switch a {
	case codenames.RedAgent:
		return CardRed
	case codenames.BlueAgent:
		return CardBlue
	case codenames.Bystander:
		return CardBystander
	case codenames.Assassin:
		return CardAssassin
	}
	return ""
}

func ownAgent(t Team) codenames.Agent {
	if t == TeamBlue {
		return codenames.BlueAgent
	}
	return codenames.RedAgent
}

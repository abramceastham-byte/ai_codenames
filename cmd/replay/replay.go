package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/bcspragu/Codenames/botproto"
	"github.com/bcspragu/Codenames/codenames"
)

// replay is a loaded move log, precomputed into one frame per event so
// stepping backwards is just an index change.
type replay struct {
	start  botproto.LogMatchStart
	frames []frame
	// truncated is true if the log has no result line, e.g. because the
	// match was interrupted.
	truncated bool
}

// frame is the game as it stood right after one log line was applied.
type frame struct {
	// event is the line just applied; nil for the starting position.
	event *botproto.LogEvent
	board []botproto.Card

	turn      int
	team      botproto.Team
	clue      *botproto.Clue
	lastGuess *botproto.LogEvent
	// forfeit is set if the current turn had a forfeit.
	forfeit  *botproto.LogEvent
	forfeits map[botproto.Team]int

	result *botproto.LogResult
}

// loadReplay reads a move log written by cmd/match. It returns an error
// naming the line for anything malformed or inconsistent with the board.
func loadReplay(r io.Reader) (*replay, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	rp := &replay{}
	lineNo := 0
	var cur frame
	sawStart := false
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("line %d: %s", lineNo, fmt.Sprintf(format, args...))
		}

		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &head); err != nil {
			return nil, fail("invalid JSON (the log may be truncated or corrupted): %v", err)
		}

		if !sawStart {
			if head.Type != botproto.LogMatchStartType {
				return nil, fail("expected the first line to be %q, got %q; is this a move log from cmd/match?", botproto.LogMatchStartType, head.Type)
			}
			if err := json.Unmarshal([]byte(line), &rp.start); err != nil {
				return nil, fail("invalid match_start line: %v", err)
			}
			if err := checkBoard(rp.start.Board); err != nil {
				return nil, fail("invalid starting board: %v", err)
			}
			sawStart = true
			cur = frame{
				board:    append([]botproto.Card(nil), rp.start.Board...),
				team:     rp.start.StartingTeam,
				forfeits: map[botproto.Team]int{},
			}
			rp.frames = append(rp.frames, cur)
			continue
		}

		if cur.result != nil {
			return nil, fail("unexpected %q line after the result line", head.Type)
		}

		if head.Type == botproto.LogResultType {
			var res botproto.LogResult
			if err := json.Unmarshal([]byte(line), &res); err != nil {
				return nil, fail("invalid result line: %v", err)
			}
			cur = cur.next()
			cur.result = &res
			rp.frames = append(rp.frames, cur)
			continue
		}

		var ev botproto.LogEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fail("invalid %s line: %v", head.Type, err)
		}
		if ev.Team != botproto.TeamRed && ev.Team != botproto.TeamBlue {
			return nil, fail("unknown team %q", ev.Team)
		}
		if ev.Turn < 1 {
			return nil, fail("invalid turn number %d", ev.Turn)
		}

		cur = cur.next()
		cur.event = &ev
		if ev.Turn != cur.turn {
			// A new turn starts: clear the previous turn's details.
			cur.turn, cur.team = ev.Turn, ev.Team
			cur.clue, cur.lastGuess, cur.forfeit = nil, nil, nil
		}

		switch ev.Type {
		case botproto.LogClue:
			if ev.Count < 1 {
				return nil, fail("clue %q has invalid count %d", ev.Word, ev.Count)
			}
			cur.clue = &botproto.Clue{Word: ev.Word, Count: ev.Count}
		case botproto.LogGuess:
			i := cardIndex(cur.board, ev.Word)
			switch {
			case i < 0:
				return nil, fail("guess %q is not on the board", ev.Word)
			case cur.board[i].Revealed:
				return nil, fail("guess %q was already revealed", ev.Word)
			case cur.board[i].Type != ev.Revealed:
				return nil, fail("guess %q revealed %q, but the starting board says it's %q", ev.Word, ev.Revealed, cur.board[i].Type)
			}
			cur.board[i].Revealed = true
			cur.board[i].RevealedBy = ev.Team
			cur.lastGuess = &ev
		case botproto.LogForfeit:
			cur.forfeit = &ev
			cur.forfeits[ev.Team]++
		case botproto.LogTurnEnd:
		default:
			return nil, fail("unknown line type %q", ev.Type)
		}
		rp.frames = append(rp.frames, cur)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading log: %w", err)
	}
	if !sawStart {
		return nil, errors.New("the log is empty")
	}
	rp.truncated = rp.frames[len(rp.frames)-1].result == nil
	return rp, nil
}

// next returns a copy of f that can be changed without affecting f.
func (f frame) next() frame {
	f.event = nil
	f.board = append([]botproto.Card(nil), f.board...)
	forfeits := make(map[botproto.Team]int, len(f.forfeits))
	for k, v := range f.forfeits {
		forfeits[k] = v
	}
	f.forfeits = forfeits
	return f
}

// left returns how many of a team's cards are still unrevealed.
func (f frame) left(t botproto.Team) int {
	want := botproto.CardRed
	if t == botproto.TeamBlue {
		want = botproto.CardBlue
	}
	n := 0
	for _, c := range f.board {
		if c.Type == want && !c.Revealed {
			n++
		}
	}
	return n
}

func checkBoard(board []botproto.Card) error {
	if len(board) != codenames.Size {
		return fmt.Errorf("got %d cards, want %d", len(board), codenames.Size)
	}
	seen := map[string]bool{}
	for _, c := range board {
		switch c.Type {
		case botproto.CardRed, botproto.CardBlue, botproto.CardBystander, botproto.CardAssassin:
		default:
			return fmt.Errorf("card %q has unknown type %q", c.Word, c.Type)
		}
		if c.Revealed {
			return fmt.Errorf("card %q is already revealed", c.Word)
		}
		w := strings.ToLower(c.Word)
		if seen[w] {
			return fmt.Errorf("card %q appears twice", c.Word)
		}
		seen[w] = true
	}
	return nil
}

func cardIndex(board []botproto.Card, word string) int {
	for i, c := range board {
		if strings.EqualFold(c.Word, word) {
			return i
		}
	}
	return -1
}

// ANSI color codes: a foreground for spymaster-only hints and a background
// for revealed cards.
var (
	fgColor = map[botproto.CardType]string{
		botproto.CardRed:       "\033[31m",
		botproto.CardBlue:      "\033[34m",
		botproto.CardBystander: "\033[33m",
		botproto.CardAssassin:  "\033[35m",
	}
	bgColor = map[botproto.CardType]string{
		botproto.CardRed:       "\033[1;37;41m",
		botproto.CardBlue:      "\033[1;37;44m",
		botproto.CardBystander: "\033[30;43m",
		botproto.CardAssassin:  "\033[1;37;40m",
	}
	tagLetter = map[botproto.CardType]string{
		botproto.CardRed:       "R",
		botproto.CardBlue:      "B",
		botproto.CardBystander: "N",
		botproto.CardAssassin:  "A",
	}
)

const colorReset = "\033[0m"

// render draws frame i as text. In spymaster view, unrevealed cards show
// their type too. Tags are drawn even with color, so the board reads without it.
func (rp *replay) render(i int, spymaster, color bool) string {
	f := rp.frames[i]
	var b strings.Builder

	view := "OPERATIVE view (revealed cards only)"
	if spymaster {
		view = "SPYMASTER view (all card types)"
	}
	fmt.Fprintf(&b, "%s (red) vs %s (blue) · seed %d · step %d/%d · %s\n\n",
		rp.start.Red.Name, rp.start.Blue.Name, rp.start.Seed, i, len(rp.frames)-1, view)

	width := 0
	for _, c := range f.board {
		width = max(width, len(c.Word))
	}
	for row := 0; row < codenames.Rows; row++ {
		for col := 0; col < codenames.Columns; col++ {
			c := f.board[row*codenames.Columns+col]
			var tag, on string
			switch {
			case c.Revealed:
				tag, on = "["+tagLetter[c.Type]+"]", bgColor[c.Type]
			case spymaster:
				tag, on = " "+strings.ToLower(tagLetter[c.Type])+" ", fgColor[c.Type]
			default:
				tag = "   "
			}
			cell := fmt.Sprintf("%s %-*s", tag, width, c.Word)
			if color && on != "" {
				cell = on + cell + colorReset
			}
			b.WriteString(cell + "  ")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n[R] red  [B] blue  [N] bystander  [A] assassin   brackets = revealed")
	if spymaster {
		b.WriteString(", lowercase = hidden")
	}
	b.WriteString("\n\n")

	if f.turn == 0 {
		fmt.Fprintf(&b, "Start · %s team goes first\n", f.team)
	} else {
		fmt.Fprintf(&b, "Turn %d · %s (%s)\n", f.turn, f.team, rp.teamName(f.team))
	}
	switch {
	case f.clue != nil:
		fmt.Fprintf(&b, "Clue:        %s %d\n", f.clue.Word, f.clue.Count)
	case f.forfeit != nil && f.forfeit.Role == botproto.RoleSpymaster:
		b.WriteString("Clue:        none (spymaster forfeited)\n")
	default:
		b.WriteString("Clue:        -\n")
	}
	if f.lastGuess != nil {
		fmt.Fprintf(&b, "Last guess:  %s → %s\n", f.lastGuess.Word, f.lastGuess.Revealed)
	} else {
		b.WriteString("Last guess:  -\n")
	}
	fmt.Fprintf(&b, "Cards left:  red %d · blue %d\n", f.left(botproto.TeamRed), f.left(botproto.TeamBlue))
	fmt.Fprintf(&b, "Forfeits:    red %d · blue %d\n", f.forfeits[botproto.TeamRed], f.forfeits[botproto.TeamBlue])
	if f.forfeit != nil {
		fmt.Fprintf(&b, "  ! %s %s (%s) forfeited: %s\n", f.forfeit.Team, f.forfeit.Role, f.forfeit.Bot, f.forfeit.Reason)
	}
	if desc := describe(f.event); desc != "" {
		fmt.Fprintf(&b, "\n> %s\n", desc)
	}

	if f.result != nil {
		b.WriteString("\n=== Result ===\n")
		if f.result.Winner == "" {
			b.WriteString("Draw")
		} else {
			fmt.Fprintf(&b, "Winner: %s (%s)", rp.teamName(f.result.Winner), f.result.Winner)
		}
		fmt.Fprintf(&b, " · %s · turns: %d", f.result.Reason, f.result.Turns)
		if f.result.Tiebreak != "" {
			fmt.Fprintf(&b, " · tiebreak: %s", f.result.Tiebreak)
		}
		b.WriteString("\n")
	} else if rp.truncated && i == len(rp.frames)-1 {
		b.WriteString("\n=== End of log ===\nThe log has no result line, so the match may have been interrupted.\n")
	}
	return b.String()
}

func describe(ev *botproto.LogEvent) string {
	if ev == nil {
		return ""
	}
	switch ev.Type {
	case botproto.LogClue:
		return fmt.Sprintf("%s spymaster gives clue %q for %d", ev.Team, ev.Word, ev.Count)
	case botproto.LogGuess:
		return fmt.Sprintf("%s operative guesses %q, revealing a %s card", ev.Team, ev.Word, ev.Revealed)
	case botproto.LogForfeit:
		return fmt.Sprintf("%s %s forfeits the turn", ev.Team, ev.Role)
	case botproto.LogTurnEnd:
		return fmt.Sprintf("%s's turn ends (%s)", ev.Team, strings.ReplaceAll(string(ev.End), "_", " "))
	}
	return ""
}

func (rp *replay) teamName(t botproto.Team) string {
	if t == botproto.TeamBlue {
		return rp.start.Blue.Name
	}
	return rp.start.Red.Name
}

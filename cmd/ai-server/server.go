package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bcspragu/Codenames/client"
	"github.com/bcspragu/Codenames/codenames"
	"github.com/bcspragu/Codenames/httperr"
	"github.com/bcspragu/Codenames/llm"
	"github.com/bcspragu/Codenames/msgs"
)

// AI is the interface that AI backends must implement.
type AI interface {
	codenames.Spymaster
	codenames.Operative
}

const (
	maxConcurrentGames = 25
)

type activePlayer struct {
	gameID codenames.GameID
}

type Server struct {
	ais               map[string]AI
	defaultBackend    string
	authSecret        string
	webServerEndpoint string
	r                 *rand.Rand

	mux *http.ServeMux

	mu            sync.Mutex
	activePlayers map[codenames.RobotID]*activePlayer

	reasoningLog     *os.File
	reasoningLogPath string
	reasoningMu      sync.Mutex
}

func newServer(ais map[string]AI, defaultBackend, authSecret, webServerEndpoint string, r *rand.Rand, reasoningLog *os.File, reasoningLogPath string) *Server {
	srv := &Server{
		ais:               ais,
		defaultBackend:    defaultBackend,
		authSecret:        authSecret,
		webServerEndpoint: webServerEndpoint,
		r:                 r,
		activePlayers:     make(map[codenames.RobotID]*activePlayer),
		reasoningLog:      reasoningLog,
		reasoningLogPath:  reasoningLogPath,
	}
	srv.initMux()
	return srv
}

// reasoningLogEntry is one JSONL record capturing why an AI backend picked a
// given clue or guess, for debugging/reviewing AI decision quality after the
// fact. It's written independently of the player-facing game log, so it can
// be cross-referenced with logs/all_games.csv later by game_id+round+team.
type reasoningLogEntry struct {
	Timestamp string           `json:"timestamp"`
	GameID    codenames.GameID `json:"game_id"`
	Round     int              `json:"round"`
	Team      codenames.Team   `json:"team"`
	Role      codenames.Role   `json:"role"`
	Backend   string           `json:"backend"`
	Action    string           `json:"action"` // "clue" | "guess"
	Detail    string           `json:"detail"`
	Reasoning string           `json:"reasoning"`
	Error     string           `json:"error,omitempty"`
	// SuspectedCompound names a board word embedded in the clue that isn't a
	// derived form of it — "airport" for AIR. These are recorded but never
	// blocked: telling a real compound from a coincidence ("justice" contains
	// "ice") needs a wordlist, and a wrongly refused clue mid-round is a
	// visible defect in the instrument. Logging them yields labeled data on
	// how often this actually fires across real games.
	SuspectedCompound string `json:"suspected_compound,omitempty"`

	// The fields below are populated for Action == "guess" only, capturing
	// the confidence-scored candidate decision (see llm.GuessWithCandidates)
	// so thresholds can be re-tuned against logged data without re-running
	// games. They're all omitempty since non-LLM backends (w2v) and older log
	// entries won't have them.
	RawResponse            string              `json:"raw_response,omitempty"`
	Candidates             []guessCandidateLog `json:"candidates,omitempty"`
	RiskiestBoardWord      string              `json:"riskiest_board_word,omitempty"`
	TopCandidateIsRiskiest bool                `json:"top_candidate_is_riskiest,omitempty"`
	MustGuess              bool                `json:"must_guess,omitempty"`
	GuessesThisTurn        int                 `json:"guesses_this_turn,omitempty"`
	ClueNumber             int                 `json:"clue_number,omitempty"`
	ThresholdApplied       float64             `json:"threshold_applied,omitempty"`
	ParseError             bool                `json:"parse_error,omitempty"`
	// CapApplied is true when at least one candidate's confidence was reduced
	// by its link_type cap (llm.GuessDecisionConfig.LinkTypeCaps) — i.e. the
	// model reported a higher confidence than its own calibration rule
	// allows. Lets a batch replay count how often the model is inflating.
	CapApplied bool `json:"cap_applied,omitempty"`
	// Correct and ActualWord are filled in once the guess resolves (from the
	// subsequent GuessGiven event), not at decision time — a guess call
	// doesn't know its own outcome yet. Correct is a pointer so "unknown yet"
	// (nil) is distinguishable from "known wrong" (false); it stays nil for a
	// pass, which has no correctness to report.
	Correct    *bool  `json:"correct,omitempty"`
	ActualWord string `json:"actual_word,omitempty"`
}

// guessCandidateLog is the logged shape of one llm.Candidate.
type guessCandidateLog struct {
	Word       string  `json:"word"`
	Confidence float64 `json:"confidence"`
	// RawConfidence is the model's reported confidence before any link_type
	// cap was applied (but after the [0,1] range clamp) — the calibration
	// signal: compare against Confidence to see how often the model is
	// inflating relative to its own link_type's cap.
	RawConfidence float64 `json:"raw_confidence"`
	Reasoning     string  `json:"reasoning"`
	LinkType      string  `json:"link_type"`
}

func (s *Server) logReasoning(e reasoningLogEntry) {
	if s.reasoningLog == nil {
		return
	}
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(e)
	if err != nil {
		log.Printf("[ERROR] failed to marshal reasoning log entry: %v", err)
		return
	}
	b = append(b, '\n')

	s.reasoningMu.Lock()
	defer s.reasoningMu.Unlock()
	if _, err := s.reasoningLog.Write(b); err != nil {
		log.Printf("[ERROR] failed to write reasoning log entry: %v", err)
	}
}

func (s *Server) initMux() {
	mux := http.NewServeMux()
	mux.HandleFunc("/join", s.handleError(s.serveJoin))
	mux.HandleFunc("/backends", s.handleError(s.serveBackends))
	mux.HandleFunc("/reasoning", s.handleError(s.serveReasoning))
	s.mux = mux
}

// serveReasoning returns the logged AI reasoning entries for a single game.
// It's only ever called server-to-server by the web server's admin-gated
// endpoint (see web/web.go), never directly by a browser, so it reuses the
// same shared-secret auth as /join and /backends.
func (s *Server) serveReasoning(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return httperr.MethodNotAllowed("call to reasoning with bad method %q", r.Method)
	}
	if r.Header.Get("Authorization") != s.authSecret {
		return httperr.Forbidden("bad auth on reasoning request").WithMessage("invalid auth")
	}

	gID := codenames.GameID(r.URL.Query().Get("game_id"))
	if gID == "" {
		return httperr.BadRequest("no game_id given").WithMessage("no game_id given")
	}

	entries, err := s.reasoningForGame(gID)
	if err != nil {
		return httperr.Internal("failed to read reasoning log: %w", err)
	}

	return jsonResp(w, struct {
		Entries []reasoningLogEntry `json:"entries"`
	}{entries})
}

// reasoningForGame scans the reasoning log file for entries belonging to
// gID. The log is small (per-deployment, dev/small-scale usage), so a full
// scan per request is acceptable and avoids needing an index.
func (s *Server) reasoningForGame(gID codenames.GameID) ([]reasoningLogEntry, error) {
	if s.reasoningLogPath == "" {
		return nil, nil
	}

	f, err := os.Open(s.reasoningLogPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open reasoning log: %w", err)
	}
	defer f.Close()

	var entries []reasoningLogEntry
	scanner := bufio.NewScanner(f)
	// Reasoning text can be long; grow the buffer past bufio's 64KB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e reasoningLogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			log.Printf("[ERROR] failed to parse reasoning log line: %v", err)
			continue
		}
		if e.GameID == gID {
			entries = append(entries, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan reasoning log: %w", err)
	}

	return entries, nil
}

func (s *Server) serveBackends(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return httperr.MethodNotAllowed("call to backends with bad method %q", r.Method)
	}
	if r.Header.Get("Authorization") != s.authSecret {
		return httperr.Forbidden("bad auth on backends request").WithMessage("invalid auth")
	}

	names := make([]string, 0, len(s.ais))
	for k := range s.ais {
		names = append(names, k)
	}
	sort.Strings(names)

	return jsonResp(w, struct {
		Backends []string `json:"backends"`
		Default  string   `json:"default"`
	}{names, s.defaultBackend})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) serveJoin(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return httperr.MethodNotAllowed("call to join with bad method %q", r.Method)
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return httperr.
			Unauthorized("no auth in join request").
			WithMessage("no auth given")
	}
	if auth != s.authSecret {
		return httperr.
			Forbidden("bad auth secret in join requesrt").
			WithMessage("invalid auth")
	}

	var req struct {
		GameID  string `json:"game_id"`
		Team    string `json:"team"`
		Role    string `json:"role"`
		Backend string `json:"backend"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return httperr.BadRequest("failed to decode join request: %w", err)
	}

	if req.GameID == "" {
		return httperr.
			BadRequest("no game ID given").
			WithMessage("no game ID given")
	}
	if req.Team == "" {
		return httperr.
			BadRequest("no team given").
			WithMessage("no team given")
	}
	if req.Role == "" {
		return httperr.
			BadRequest("no role given").
			WithMessage("no role given")
	}
	role, ok := codenames.ToRole(req.Role)
	if !ok {
		return httperr.
			BadRequest("bad role given").
			WithMessage("bad role given")
	}
	team, ok := codenames.ToTeam(req.Team)
	if !ok {
		return httperr.
			BadRequest("bad team given").
			WithMessage("bad team given")
	}
	gID := codenames.GameID(req.GameID)

	backendName := req.Backend
	if backendName == "" {
		backendName = s.defaultBackend
	}
	ai, ok := s.ais[backendName]
	if !ok {
		return httperr.
			BadRequest("backend %q is not enabled on this server", backendName).
			WithMessage("requested AI backend is not available")
	}

	// We need a client-per-bot because it has its own cookie jar for auth
	c, err := client.New(s.webServerEndpoint)
	if err != nil {
		return httperr.Internal("failed to init Codenames client: %w", err)
	}

	// The web server assigns every player (human or AI) a random generated
	// name, so we don't send one.
	pID, err := c.CreateUser("", codenames.PlayerTypeRobot)
	if err != nil {
		return httperr.Internal("failed to create AI user: %w", err)
	}
	rID := codenames.RobotID(pID)

	if err := c.JoinGame(gID); err != nil {
		return httperr.Internal("failed to join game %q: %w", gID, err)
	}

	if err := c.AssignRole(gID, team, role); err != nil {
		return httperr.Internal("failed to assign role %q: %w", gID, err)
	}

	s.mu.Lock()
	if len(s.activePlayers) >= maxConcurrentGames {
		s.mu.Unlock()
		return httperr.
			Teapot("can't join a game when we already have %d active games", len(s.activePlayers)).
			WithMessage("too many active AIs, try again later")
	}
	s.activePlayers[rID] = &activePlayer{gameID: gID}
	s.mu.Unlock()
	log.Printf("Created player %q (backend=%s) in game %q", rID, backendName, gID)

	// Background the process of playing.
	go func() {
		defer s.unlockPlayer(rID)

		s.playGame(ai, backendName, c, gID, rID)
	}()

	return jsonResp(w, struct {
		RobotID string `json:"robot_id"`
		Success bool   `json:"success"`
	}{string(rID), true})
}

func (s *Server) unlockPlayer(rID codenames.RobotID) {
	s.mu.Lock()
	delete(s.activePlayers, rID)
	s.mu.Unlock()
}

// opposingTeam returns the other team.
func opposingTeam(t codenames.Team) codenames.Team {
	if t == codenames.RedTeam {
		return codenames.BlueTeam
	}
	return codenames.RedTeam
}

func (s *Server) playGame(ai AI, backendName string, c *client.Client, gID codenames.GameID, rID codenames.RobotID) {
	var (
		role     codenames.Role
		team     codenames.Team
		lastClue *codenames.Clue
		// activeTeam tracks whose turn it currently is, so OnPlayerVote can
		// tell a live teammate vote from stale ones (e.g. a tentative tap
		// left over from before the last guess resolved).
		activeTeam codenames.Team
		// teamClueCount mirrors the frontend's round-number logic
		// (round = max of each team's clue count), so reasoning log entries can
		// be cross-referenced with logs/all_games.csv by game_id+round+team.
		teamClueCount = map[codenames.Team]int{}
		// guessesThisTurn and revealedThisTurn track our own team's progress on
		// the current clue, for the candidate-operative prompt's context and
		// its bonus-guess threshold. Both reset whenever our team gets a new
		// clue.
		guessesThisTurn  int
		revealedThisTurn []string
		// pending holds the log entry for our most recent guess, awaiting the
		// GuessGiven event that reveals whether it was correct.
		pending *pendingGuess
	)

	// predictedClueRound returns the round number a new clue from t will get
	// once confirmed. Safe because clue-giving is turn-serialized by game
	// rules — no other clue event can land before this one is confirmed.
	predictedClueRound := func(t codenames.Team) int {
		return max(teamClueCount[t]+1, teamClueCount[opposingTeam(t)])
	}

	err := c.ListenForUpdates(gID, client.WSHooks{
		OnConnect: func() {
			// TODO(bcspragu): Decide if we need to do anything once we connect.
		},
		OnStart: func(gs *msgs.GameStart) {
			for _, p := range gs.Players {
				// Match on the opaque ID only — the web server strips the
				// human/robot distinction from player-facing messages, so
				// IsRobot would never match here.
				if !p.PlayerID.SameID(string(rID)) {
					continue
				}
				role = p.Role
				team = p.Team
				break
			}

			// If we can't find ourselves in the player list we have no role,
			// so every hook below silently does nothing and the game stalls.
			// Shout about it rather than hanging quietly.
			if role == codenames.NoRole {
				log.Printf("[ERROR] robot %q not found in player list for game %q — it will not play", rID, gID)
				return
			}
			log.Printf("Game %q started; I'm the %s %s", gID, team, role)
			activeTeam = gs.Game.State.ActiveTeam

			if role == codenames.SpymasterRole && gs.Game.State.ActiveTeam == team {
				rc := reasoningCtx{gameID: gID, backend: backendName, team: team, round: predictedClueRound(team)}
				clue, err := s.giveClue(ai, gs.Game.State.Board, toAgent(team), rc)
				if err != nil {
					log.Printf("[ERROR] failed to make a clue: %v", err)
					return
				}

				if err := c.GiveClue(gID, clue); err != nil {
					log.Printf("[ERROR] failed to give clue: %v", err)
					return
				}
			}
		},
		OnClueGiven: func(cg *msgs.ClueGiven) {
			teamClueCount[cg.Team]++
			activeTeam = cg.Game.State.ActiveTeam

			if cg.Team == team {
				lastClue = cg.Clue
				// A new clue for our team starts a fresh turn.
				guessesThisTurn = 0
				revealedThisTurn = nil
				pending = nil
			}

			if role != codenames.OperativeRole || cg.Team != team {
				// fmt.Printf("Clue was given, but I'm a/an %q on team %q\n", role, team)
				return
			}
			log.Printf("Clue %q was given, and I'm guessing!", cg.Clue)

			round := max(teamClueCount[codenames.RedTeam], teamClueCount[codenames.BlueTeam])
			rc := reasoningCtx{gameID: gID, backend: backendName, team: team, round: round}
			guess, pg, err := s.guess(ai, cg.Game.State.Board, cg.Clue, true /* mustGuess */, guessesThisTurn, revealedThisTurn, rc)
			if err != nil {
				log.Printf("[ERROR] failed to make a guess for clue %+v: %v", cg.Clue, err)
				return
			}
			pending = pg

			if err := c.GiveGuess(gID, guess, true /* confirmed */); err != nil {
				log.Printf("[ERROR] failed to give guess %q for clue %+v: %v", guess, cg.Clue, err)
				return
			}
		},
		OnGuessGiven: func(gg *msgs.GuessGiven) {
			activeTeam = gg.Game.State.ActiveTeam

			// This event reports the outcome of our own just-submitted guess
			// (correct/wrong/pass) — finish and write its pending log entry now
			// that the result is known. Must happen before the branches below,
			// since a correct guess also feeds revealedThisTurn for the next one.
			if gg.Team == team && role == codenames.OperativeRole && pending != nil {
				s.logOutcome(pending, gg.RevealedCard, toAgent(team))
				pending = nil
				if gg.RevealedCard != nil {
					guessesThisTurn++
					if gg.RevealedCard.Agent == toAgent(team) {
						revealedThisTurn = append(revealedThisTurn, gg.RevealedCard.Codename)
					}
				}
			}

			// We only want to formulate a clue when the *other* team has just
			// finished guessing.
			if gg.Team != team && !gg.CanKeepGuessing && role == codenames.SpymasterRole {

				rc := reasoningCtx{gameID: gID, backend: backendName, team: team, round: predictedClueRound(team)}
				clue, err := s.giveClue(ai, gg.Game.State.Board, toAgent(team), rc)
				if err != nil {
					log.Printf("[ERROR] failed to make a clue: %v", err)
					return
				}

				if err := c.GiveClue(gID, clue); err != nil {
					log.Printf("[ERROR] failed to give clue: %v", err)
					return
				}

				return
			}

			if gg.Team == team && gg.CanKeepGuessing && role == codenames.OperativeRole {
				round := max(teamClueCount[codenames.RedTeam], teamClueCount[codenames.BlueTeam])
				rc := reasoningCtx{gameID: gID, backend: backendName, team: team, round: round}
				guess, pg, err := s.guess(ai, gg.Game.State.Board, lastClue, false /* mustGuess */, guessesThisTurn, revealedThisTurn, rc)
				if err != nil {
					log.Printf("[ERROR] failed to make a guess for clue %+v: %v", lastClue, err)
					return
				}
				pending = pg

				if err := c.GiveGuess(gID, guess, true /* confirmed */); err != nil {
					log.Printf("[ERROR] failed to give guess %q for clue %+v: %v", guess, lastClue, err)
					return
				}

				return
			}
		},
		OnPlayerVote: func(pv *msgs.PlayerVote) {
			// Consensus requires every operative on the team to agree, and we
			// otherwise only ever cast one independent vote per clue/guess
			// event (above). If a human teammate is also an operative, our
			// one guess has to exactly match theirs or the vote never
			// resolves. Rather than requiring that coincidence, defer to a
			// teammate's confirmed choice once they've locked one in.
			if role != codenames.OperativeRole || activeTeam != team || !pv.Confirmed {
				return
			}
			if pv.PlayerID.SameID(string(rID)) {
				// Don't react to the vote we ourselves just cast.
				return
			}
			log.Printf("Teammate voted %q, seconding it", pv.Guess)
			if err := c.GiveGuess(gID, pv.Guess, true /* confirmed */); err != nil {
				log.Printf("[ERROR] failed to second teammate's guess %q: %v", pv.Guess, err)
			}
		},
	})
	if err != nil {
		log.Printf("[ERROR] error listening for updates in game %q: %v", gID, err)
	}
}

// reasoningCtx carries the context needed to log why an AI backend picked a
// given clue/guess, without bloating giveClue/guess's parameter lists.
type reasoningCtx struct {
	gameID  codenames.GameID
	backend string
	team    codenames.Team
	round   int
}

// reasoningSpymaster is implemented by AI backends that can explain why they
// picked a given clue.
type reasoningSpymaster interface {
	GiveClueWithReasoning(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, string, error)
}

func (s *Server) giveClue(ai AI, b *codenames.Board, agent codenames.Agent, rc reasoningCtx) (*codenames.Clue, error) {
	start := time.Now()

	var (
		clue      *codenames.Clue
		reasoning string
		err       error
	)
	if rs, ok := ai.(reasoningSpymaster); ok {
		clue, reasoning, err = rs.GiveClueWithReasoning(b, agent)
	} else {
		clue, err = ai.GiveClue(b, agent)
	}
	if err != nil {
		log.Printf("[ERROR] AI failed to make a clue: %v", err)
		clue = &codenames.Clue{
			Word:  "???",
			Count: 1,
		}
	}

	// Second-tier check: recorded for later analysis, deliberately not enforced.
	suspect, _ := codenames.SuspectedCompound(clue.Word, b.Cards)
	if suspect != "" {
		log.Printf("[WARN] clue %q may be a compound of board word %q (allowed, logged)", clue.Word, suspect)
	}

	s.logReasoning(reasoningLogEntry{
		GameID:            rc.gameID,
		Round:             rc.round,
		Team:              rc.team,
		Role:              codenames.SpymasterRole,
		Backend:           rc.backend,
		Action:            "clue",
		Detail:            clue.String(),
		Reasoning:         reasoning,
		Error:             errString(err),
		SuspectedCompound: suspect,
	})

	humanThinkDelay(start, 8*time.Second, 25*time.Second)
	return clue, nil
}

// errString returns "" for a nil error, and err.Error() otherwise — handy for
// putting an optional error into a log entry.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// humanThinkDelay sleeps until the total time elapsed since start falls
// somewhere in [min, max), so AI responses don't arrive at inhuman speed.
func humanThinkDelay(start time.Time, min, max time.Duration) {
	target := min + time.Duration(rand.Int63n(int64(max-min)))
	if remaining := target - time.Since(start); remaining > 0 {
		time.Sleep(remaining)
	}
}

func toAgent(team codenames.Team) codenames.Agent {
	switch team {
	case codenames.BlueTeam:
		return codenames.BlueAgent
	case codenames.RedTeam:
		return codenames.RedAgent
	default:
		return codenames.UnknownAgent
	}
}

// candidateOperative is implemented by AI backends that report ranked,
// confidence-scored candidates and let Go threshold logic decide whether to
// guess or pass (see llm.GuessWithCandidates).
type candidateOperative interface {
	GuessWithCandidates(b *codenames.Board, c *codenames.Clue, mustGuess bool, guessesThisTurn int, revealedHistory []string) (*llm.GuessResult, error)
}

// passingOperative is implemented by AI backends that can decline to guess
// (by returning codenames.PassGuess) when passing is allowed.
type passingOperative interface {
	GuessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, error)
}

// reasoningOperative is implemented by AI backends that can both decline to
// guess and explain why they picked a given guess (or pass).
type reasoningOperative interface {
	GuessOrPassWithReasoning(b *codenames.Board, c *codenames.Clue, mustGuess bool) (guess string, reasoning string, err error)
}

// pendingGuess carries the just-decided guess's log entry from guess() to the
// point where its outcome (correct/wrong/revealed word) becomes known from
// the subsequent GuessGiven event, since a guess call can't know its own
// result yet. logOutcome fills in Correct/ActualWord and writes it.
type pendingGuess struct {
	entry reasoningLogEntry
}

// logOutcome finishes and writes the log entry for a previously-decided
// guess, once its result is known. revealedCard is nil for a pass (no card
// was revealed) or for a fallback random guess where the caller couldn't
// determine the card synchronously; ourAgent is this bot's team's agent, used
// to judge correctness.
func (s *Server) logOutcome(pg *pendingGuess, revealedCard *codenames.Card, ourAgent codenames.Agent) {
	if pg == nil {
		return
	}
	if revealedCard != nil {
		correct := revealedCard.Agent == ourAgent
		pg.entry.Correct = &correct
		pg.entry.ActualWord = revealedCard.Codename
	}
	s.logReasoning(pg.entry)
}

// guess asks the AI for a guess. mustGuess is true on the first guess after a
// clue — Codenames requires at least one guess per clue — and false on
// follow-up guesses, where the AI may pass. A pass is sent to the web server
// as an empty guess, which ends the turn. guessesThisTurn and revealedHistory
// are only meaningful to a candidateOperative backend; other backends ignore
// them. The returned *pendingGuess should be handed to logOutcome once this
// guess's result is known (see the GuessGiven handler in playGame).
func (s *Server) guess(ai AI, b *codenames.Board, clue *codenames.Clue, mustGuess bool, guessesThisTurn int, revealedHistory []string, rc reasoningCtx) (string, *pendingGuess, error) {
	start := time.Now()
	entry := reasoningLogEntry{
		GameID:  rc.gameID,
		Round:   rc.round,
		Team:    rc.team,
		Role:    codenames.OperativeRole,
		Backend: rc.backend,
		Action:  "guess",
	}

	var (
		guess string
		err   error
	)
	switch v := ai.(type) {
	case candidateOperative:
		var res *llm.GuessResult
		res, err = v.GuessWithCandidates(b, clue, mustGuess, guessesThisTurn, revealedHistory)
		if res != nil {
			guess = res.Guess
			entry.Detail = res.Guess
			entry.RawResponse = res.RawResponse
			entry.RiskiestBoardWord = res.RiskiestBoardWord
			entry.TopCandidateIsRiskiest = res.TopCandidateIsRiskiest
			entry.MustGuess = res.MustGuess
			entry.GuessesThisTurn = res.GuessesThisTurn
			entry.ClueNumber = res.ClueNumber
			entry.ThresholdApplied = res.ThresholdApplied
			entry.ParseError = res.ParseError
			entry.CapApplied = res.CapApplied
			entry.Candidates = make([]guessCandidateLog, len(res.Candidates))
			for i, c := range res.Candidates {
				entry.Candidates[i] = guessCandidateLog{Word: c.Word, Confidence: c.Confidence, RawConfidence: c.RawConfidence, Reasoning: c.Reasoning, LinkType: c.LinkType}
			}
			entry.Reasoning = renderGuessReasoning(res)
		}
	case reasoningOperative:
		guess, entry.Reasoning, err = v.GuessOrPassWithReasoning(b, clue, mustGuess)
		entry.Detail = guess
	case passingOperative:
		guess, err = v.GuessOrPass(b, clue, mustGuess)
		entry.Detail = guess
	default:
		guess, err = ai.Guess(b, clue)
		entry.Detail = guess
	}
	entry.Error = errString(err)

	pg := &pendingGuess{entry: entry}

	if err != nil || guess == "" {
		log.Printf("[ERROR] AI failed to make a guess: %v", err)
		guess, err = s.guessRandomly(b)
	} else if guess == codenames.PassGuess {
		guess = ""
	}
	humanThinkDelay(start, 3*time.Second, 15*time.Second)
	return guess, pg, err
}

// renderGuessReasoning turns a candidate operative's structured decision into
// the human-readable text the admin UI's Reasoning column expects.
func renderGuessReasoning(res *llm.GuessResult) string {
	if res.ParseError {
		return "no reasoning available: every attempt failed to produce parseable candidates"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Threshold: %.2f (mustGuess=%v)\n", res.ThresholdApplied, res.MustGuess)
	fmt.Fprintf(&sb, "Riskiest board word: %s (top candidate is riskiest: %v)\n\nCandidates:\n", res.RiskiestBoardWord, res.TopCandidateIsRiskiest)
	for _, c := range res.Candidates {
		if c.Confidence < c.RawConfidence {
			fmt.Fprintf(&sb, "- %s (confidence %.2f, capped from %.2f, %s): %s\n", c.Word, c.Confidence, c.RawConfidence, c.LinkType, c.Reasoning)
		} else {
			fmt.Fprintf(&sb, "- %s (confidence %.2f, %s): %s\n", c.Word, c.Confidence, c.LinkType, c.Reasoning)
		}
	}
	return strings.TrimSpace(sb.String())
}

func (s *Server) guessRandomly(b *codenames.Board) (string, error) {
	unused := codenames.Unused(b.Cards)
	if len(unused) == 0 {
		return "", errors.New("no available cards left on the board")
	}

	return unused[s.r.Intn(len(unused))].Codename, nil
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (s *Server) handleError(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		log.Println(err)

		code, userMsg := httperr.Extract(err)
		http.Error(w, userMsg, code)
	}
}

func jsonResp(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return httperr.Internal("failed to encode response for %+v of type %T: %w", v, v, err)
	}

	return nil
}

package botproto

import (
	"context"
	"math/rand"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bcspragu/Codenames/boardgen"
	"github.com/bcspragu/Codenames/codenames"
)

func requirePython(t *testing.T) string {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	return py
}

func examplePlayer(py, name string) Player {
	return Player{
		Name:      name,
		Spymaster: []string{py, "../examples/python/spymaster.py"},
		Operative: []string{py, "../examples/python/operative.py"},
	}
}

func TestRunMatchExampleBots(t *testing.T) {
	py := requirePython(t)
	for seed := int64(0); seed < 3; seed++ {
		board := boardgen.New(codenames.RedTeam, rand.New(rand.NewSource(seed)))
		res, err := RunMatch(context.Background(), examplePlayer(py, "a"), examplePlayer(py, "b"), board, codenames.RedTeam, Options{})
		if err != nil {
			t.Fatalf("seed %d: RunMatch: %v", seed, err)
		}
		if len(res.Events) != 0 {
			t.Errorf("seed %d: example bots had failures: %+v", seed, res.Events)
		}
		if len(res.History) == 0 || len(res.History) > MaxTurns {
			t.Fatalf("seed %d: got %d turns", seed, len(res.History))
		}
		last := res.History[len(res.History)-1]
		if len(res.History) < MaxTurns && (last.End != EndGameOver || res.Winner == "") {
			t.Errorf("seed %d: game ended early without a winner: %+v, last turn %+v", seed, res, last)
		}
		for i, turn := range res.History {
			if want := []Team{TeamRed, TeamBlue}[i%2]; turn.Team != want {
				t.Errorf("seed %d: turn %d played by %q, want %q", seed, i, turn.Team, want)
			}
			if turn.Clue == nil || len(turn.Guesses) > turn.Clue.Count+1 {
				t.Errorf("seed %d: bad turn %d: %+v", seed, i, turn)
			}
		}
		t.Logf("seed %d: %q won in %d turns (%s)", seed, res.Winner, len(res.History), res.Reason)
	}
}

func TestRunMatchForfeits(t *testing.T) {
	py := requirePython(t)
	script := func(code string) []string { return []string{py, "-c", code} }
	good := examplePlayer(py, "good")

	tests := []struct {
		name    string
		bad     Player
		wantEnd TurnEnd
	}{
		{"bad json", Player{Name: "x", Spymaster: script(`print("not json")`), Operative: good.Operative}, EndSpymasterForfeit},
		{"illegal clue", Player{Name: "x", Spymaster: script(`
import json, sys
req = json.load(sys.stdin)
print(json.dumps({"clue": req["board"][0]["word"], "count": 1}))`), Operative: good.Operative}, EndSpymasterForfeit},
		{"timeout", Player{Name: "x", Spymaster: script(`import time; time.sleep(10)`), Operative: good.Operative}, EndSpymasterForfeit},
		{"crash", Player{Name: "x", Spymaster: script(`import sys; print("boom", file=sys.stderr); sys.exit(1)`), Operative: good.Operative}, EndSpymasterForfeit},
		{"guess not on board", Player{
			Name:      "x",
			Spymaster: good.Spymaster,
			Operative: script(`print('{"guesses": ["notaboardword"]}')`),
		}, EndOperativeForfeit},
		{"no guesses", Player{
			Name:      "x",
			Spymaster: good.Spymaster,
			Operative: script(`print('{"guesses": []}')`),
		}, EndOperativeForfeit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := boardgen.New(codenames.RedTeam, rand.New(rand.NewSource(1)))
			res, err := RunMatch(context.Background(), tt.bad, good, board, codenames.RedTeam, Options{MoveTimeout: 2 * time.Second, MaxTurns: 2})
			if err != nil {
				t.Fatalf("RunMatch: %v", err)
			}
			first := res.History[0]
			if first.End != tt.wantEnd {
				t.Fatalf("first turn ended %q, want %q", first.End, tt.wantEnd)
			}
			if tt.wantEnd == EndStopped {
				return
			}
			if len(res.Events) == 0 || res.Events[0].Team != TeamRed {
				t.Fatalf("expected a logged failure for red, got %+v", res.Events)
			}
			if tt.name == "crash" && res.Events[0].Stderr != "boom\n" {
				t.Errorf("stderr = %q, want %q", res.Events[0].Stderr, "boom\n")
			}
		})
	}
}

func TestTurnLimitTiebreak(t *testing.T) {
	py := requirePython(t)
	crashing := Player{
		Name:      "crashing",
		Spymaster: []string{py, "-c", "import sys; sys.exit(1)"},
		Operative: []string{py, "-c", "import sys; sys.exit(1)"},
	}
	// The stalling team gives legal clues but never guesses, so it forfeits
	// every turn without revealing cards.
	stalling := Player{
		Name:      "stalling",
		Spymaster: examplePlayer(py, "").Spymaster,
		Operative: []string{py, "-c", `print('{"guesses": []}')`},
	}
	// The steady team always guesses the first unrevealed word, so it never
	// forfeits.
	steady := Player{
		Name:      "steady",
		Spymaster: examplePlayer(py, "").Spymaster,
		Operative: []string{py, "-c", `
import json, sys
req = json.load(sys.stdin)
print(json.dumps({"guesses": [next(c["word"] for c in req["board"] if not c["revealed"])]}))`},
	}

	tests := []struct {
		name         string
		red, blue    Player
		wantWinner   Team
		wantTiebreak string
	}{
		{"crashing red loses", crashing, steady, TeamBlue, TiebreakForfeits},
		{"crashing blue loses", steady, crashing, TeamRed, TiebreakForfeits},
		// Equal forfeits and no cards revealed: a draw.
		{"no cards revealed", stalling, stalling, "", TiebreakNoCards},
		// No forfeits; decided by cards left. The winner depends on the board.
		{"cards left", steady, steady, cardsLeftWinner(safeBoard()), TiebreakCardsLeft},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := RunMatch(context.Background(), tt.red, tt.blue, safeBoard(), codenames.RedTeam, Options{MaxTurns: 4})
			if err != nil {
				t.Fatalf("RunMatch: %v", err)
			}
			if res.Winner != tt.wantWinner || res.Tiebreak != tt.wantTiebreak {
				t.Errorf("got winner %q by %q (%s), want %q by %q", res.Winner, res.Tiebreak, res.Reason, tt.wantWinner, tt.wantTiebreak)
			}
		})
	}
}

// safeBoard returns a board whose first four cards aren't the assassin, so
// the steady teams' guesses in a 4-turn game can't end it early.
func safeBoard() *codenames.Board {
	for seed := int64(1); ; seed++ {
		b := boardgen.New(codenames.RedTeam, rand.New(rand.NewSource(seed)))
		if !slices.ContainsFunc(b.Cards[:4], func(c codenames.Card) bool { return c.Agent == codenames.Assassin }) {
			return b
		}
	}
}

// cardsLeftWinner is who wins a 4-turn steady-vs-steady game on b, where
// the first four cards get revealed.
func cardsLeftWinner(b *codenames.Board) Team {
	left := map[codenames.Agent]int{codenames.RedAgent: 9, codenames.BlueAgent: 8}
	for _, c := range b.Cards[:4] {
		left[c.Agent]--
	}
	switch {
	case left[codenames.RedAgent] < left[codenames.BlueAgent]:
		return TeamRed
	case left[codenames.BlueAgent] < left[codenames.RedAgent]:
		return TeamBlue
	}
	return ""
}

func TestRunMatchChecksCommands(t *testing.T) {
	py := requirePython(t)
	good := examplePlayer(py, "good")
	board := safeBoard()

	tests := []struct {
		name    string
		bad     Player
		wantErr string
	}{
		{"missing program", Player{Name: "x", Spymaster: []string{"no-such-program-xyz"}, Operative: good.Operative}, `team "x" spymaster: program "no-such-program-xyz" not found`},
		{"missing script", Player{Name: "x", Spymaster: good.Spymaster, Operative: []string{py, "nope.py"}}, `team "x" operative: file "nope.py" not found`},
		{"missing relative program", Player{Name: "x", Spymaster: []string{"./bot"}, Operative: good.Operative, Dir: t.TempDir()}, `program "./bot" not found`},
		{"no command", Player{Name: "x", Spymaster: good.Spymaster}, `team "x" operative: no command given`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RunMatch(context.Background(), tt.bad, good, board, codenames.RedTeam, Options{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// Scripts are found relative to a player's Dir.
func TestRunMatchPlayerDir(t *testing.T) {
	py := requirePython(t)
	ex, err := filepath.Abs("../examples/python")
	if err != nil {
		t.Fatal(err)
	}
	p := Player{Name: "dir", Spymaster: []string{py, "spymaster.py"}, Operative: []string{py, "operative.py"}, Dir: ex}
	res, err := RunMatch(context.Background(), p, p, safeBoard(), codenames.RedTeam, Options{MaxTurns: 2})
	if err != nil {
		t.Fatalf("RunMatch: %v", err)
	}
	if len(res.Events) != 0 {
		t.Errorf("bots in Dir failed: %+v", res.Events)
	}
}

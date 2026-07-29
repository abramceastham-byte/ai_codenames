package game

import (
	"errors"
	"strconv"
	"testing"

	"github.com/bcspragu/Codenames/codenames"
)

// TestGamePlay drives a full game of Codenames through the Move() API,
// exercising clue-giving, guessing (including multi-guess turns and
// passing), turn switching, and win detection.
func TestGamePlay(t *testing.T) {
	g, err := New(testBoard(), codenames.RedTeam, testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Red's spymaster gives a clue for 2 words.
	state, status, err := g.Move(&Move{
		Action:   ActionGiveClue,
		Team:     codenames.RedTeam,
		GiveClue: &codenames.Clue{Word: "red-things", Count: 2},
	})
	if err != nil {
		t.Fatalf("Move(clue): %v", err)
	}
	if status != codenames.Playing {
		t.Fatalf("status after clue = %q, want %q", status, codenames.Playing)
	}
	if state.ActiveRole != codenames.OperativeRole {
		t.Fatalf("ActiveRole after clue = %q, want %q", state.ActiveRole, codenames.OperativeRole)
	}
	if state.NumGuessesLeft != 2 {
		t.Fatalf("NumGuessesLeft after clue = %d, want 2", state.NumGuessesLeft)
	}

	// First (correct) guess: a red card, with a guess remaining, so the turn
	// continues.
	state, status, err = g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r0"})
	if err != nil {
		t.Fatalf("Move(guess r0): %v", err)
	}
	if status != codenames.Playing {
		t.Fatalf("status after correct guess = %q, want %q", status, codenames.Playing)
	}
	if state.ActiveRole != codenames.OperativeRole || state.ActiveTeam != codenames.RedTeam {
		t.Fatalf("after 1st correct guess, active = %q %q, want %q %q", state.ActiveTeam, state.ActiveRole, codenames.RedTeam, codenames.OperativeRole)
	}
	if state.NumGuessesLeft != 1 {
		t.Fatalf("NumGuessesLeft after 1st correct guess = %d, want 1", state.NumGuessesLeft)
	}

	// Second (correct) guess: uses up the last guess from the clue, so the
	// turn ends automatically even though the guess was correct.
	state, status, err = g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r1"})
	if err != nil {
		t.Fatalf("Move(guess r1): %v", err)
	}
	if status != codenames.Playing {
		t.Fatalf("status after 2nd correct guess = %q, want %q", status, codenames.Playing)
	}
	if state.ActiveTeam != codenames.BlueTeam || state.ActiveRole != codenames.SpymasterRole {
		t.Fatalf("after using up guesses, active = %q %q, want %q %q", state.ActiveTeam, state.ActiveRole, codenames.BlueTeam, codenames.SpymasterRole)
	}

	// Blue gives a clue and immediately passes without guessing.
	if _, _, err := g.Move(&Move{
		Action:   ActionGiveClue,
		Team:     codenames.BlueTeam,
		GiveClue: &codenames.Clue{Word: "blue-things", Count: 1},
	}); err != nil {
		t.Fatalf("Move(blue clue): %v", err)
	}
	state, status, err = g.Move(&Move{Action: ActionGuess, Team: codenames.BlueTeam, Guess: ""})
	if err != nil {
		t.Fatalf("Move(blue pass): %v", err)
	}
	if status != codenames.Playing {
		t.Fatalf("status after pass = %q, want %q", status, codenames.Playing)
	}
	if state.ActiveTeam != codenames.RedTeam || state.ActiveRole != codenames.SpymasterRole {
		t.Fatalf("after pass, active = %q %q, want %q %q", state.ActiveTeam, state.ActiveRole, codenames.RedTeam, codenames.SpymasterRole)
	}

	// Play out the rest of Red's cards to win the game: r2..r8 (7 more
	// correct guesses), one clue+guess pair at a time.
	for _, codename := range []string{"r2", "r3", "r4", "r5", "r6", "r7", "r8"} {
		if _, _, err := g.Move(&Move{
			Action:   ActionGiveClue,
			Team:     codenames.RedTeam,
			GiveClue: &codenames.Clue{Word: "another-red-thing", Count: 1},
		}); err != nil {
			t.Fatalf("Move(clue before %q): %v", codename, err)
		}

		state, status, err = g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: codename})
		if err != nil {
			t.Fatalf("Move(guess %q): %v", codename, err)
		}
	}

	if status != codenames.Finished {
		t.Fatalf("status after revealing all red cards = %q, want %q", status, codenames.Finished)
	}
	over, winner := g.GameOver()
	if !over {
		t.Fatal("GameOver() = false after revealing all red cards, want true")
	}
	if winner != codenames.RedTeam {
		t.Fatalf("winner = %q, want %q", winner, codenames.RedTeam)
	}
}

// TestGamePlayInvalidMoves checks that out-of-turn and malformed moves are
// rejected, and that a rejected guess doesn't consume any of the current
// clue's guess budget.
func TestGamePlayInvalidMoves(t *testing.T) {
	t.Run("guess before any clue given", func(t *testing.T) {
		g := newTestGame(t)
		if _, _, err := g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r0"}); err == nil {
			t.Fatal("Move(guess) before a clue was given: got nil error, want error")
		}
	})

	t.Run("clue given during operative's turn", func(t *testing.T) {
		g := newTestGame(t)
		if _, _, err := g.Move(&Move{
			Action:   ActionGiveClue,
			Team:     codenames.RedTeam,
			GiveClue: &codenames.Clue{Word: "red-things", Count: 2},
		}); err != nil {
			t.Fatalf("Move(clue): %v", err)
		}
		if _, _, err := g.Move(&Move{Action: ActionGiveClue, Team: codenames.RedTeam, GiveClue: &codenames.Clue{Word: "x", Count: 1}}); err == nil {
			t.Fatal("Move(clue) during operative's turn: got nil error, want error")
		}
	})

	t.Run("clue that is a board word", func(t *testing.T) {
		g := newTestGame(t)
		if _, _, err := g.Move(&Move{
			Action:   ActionGiveClue,
			Team:     codenames.RedTeam,
			GiveClue: &codenames.Clue{Word: "r0", Count: 2},
		}); err == nil {
			t.Fatal("Move(clue) naming a board word: got nil error, want error")
		}

		// The rejected clue must not have handed the turn to the operative,
		// or the team would be stuck guessing against a clue that was refused.
		if g.state.ActiveRole != codenames.SpymasterRole {
			t.Fatalf("after a rejected clue, ActiveRole = %q, want %q", g.state.ActiveRole, codenames.SpymasterRole)
		}
		if len(g.state.Clues) != 0 {
			t.Fatalf("after a rejected clue, len(Clues) = %d, want 0", len(g.state.Clues))
		}

		// A legal clue still works afterwards.
		if _, _, err := g.Move(&Move{
			Action:   ActionGiveClue,
			Team:     codenames.RedTeam,
			GiveClue: &codenames.Clue{Word: "red-things", Count: 2},
		}); err != nil {
			t.Fatalf("Move(clue) after a rejected clue: %v", err)
		}
	})

	t.Run("clue that is a derived form of a board word", func(t *testing.T) {
		// The shared test board uses two-letter codenames, which are below the
		// root-length floor, so this needs a board with a real word on it.
		g := NewForMove(&codenames.GameState{
			ActiveTeam:   codenames.RedTeam,
			ActiveRole:   codenames.SpymasterRole,
			StartingTeam: codenames.RedTeam,
			Board: &codenames.Board{Cards: []codenames.Card{
				{Codename: "king", Agent: codenames.RedAgent},
				{Codename: "ice", Agent: codenames.BlueAgent},
			}},
		})

		for _, word := range []string{"kings", "kingdom", "KINGLY", "icy", "icing"} {
			if _, _, err := g.Move(&Move{
				Action:   ActionGiveClue,
				Team:     codenames.RedTeam,
				GiveClue: &codenames.Clue{Word: word, Count: 1},
			}); err == nil {
				t.Errorf("Move(clue %q): got nil error, want error", word)
			}
		}

		// Containment alone is not a conflict — these must all be playable.
		for _, word := range []string{"nice", "justice", "bring", "kite"} {
			if _, _, err := g.Move(&Move{
				Action:   ActionGiveClue,
				Team:     codenames.RedTeam,
				GiveClue: &codenames.Clue{Word: word, Count: 1},
			}); err != nil {
				t.Errorf("Move(clue %q): %v, want nil", word, err)
			}
			// Each accepted clue hands the turn over, so reset for the next.
			g.state.ActiveRole = codenames.SpymasterRole
		}
	})

	t.Run("clue that is a board word in a different case", func(t *testing.T) {
		g := newTestGame(t)
		// Human clues arrive upper-cased from the web handler, so the check
		// has to be case-insensitive to catch them at all.
		if _, _, err := g.Move(&Move{
			Action:   ActionGiveClue,
			Team:     codenames.RedTeam,
			GiveClue: &codenames.Clue{Word: "R0", Count: 1},
		}); err == nil {
			t.Fatal("Move(clue) naming an upper-cased board word: got nil error, want error")
		}
	})

	t.Run("guess for a word not on the board", func(t *testing.T) {
		g := newTestGame(t)
		mustGiveClue(t, g, codenames.RedTeam, "red-things", 1)
		if _, _, err := g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "not-a-card"}); err == nil {
			t.Fatal("Move(guess) for unknown card: got nil error, want error")
		}
		// The rejected guess shouldn't have consumed the guess budget: a
		// follow-up valid guess should still succeed and count as the
		// clue's one and only guess.
		state, _, err := g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r0"})
		if err != nil {
			t.Fatalf("Move(guess r0) after rejected guess: %v", err)
		}
		if state.ActiveTeam != codenames.BlueTeam {
			t.Fatalf("after using up the clue's only guess, ActiveTeam = %q, want %q", state.ActiveTeam, codenames.BlueTeam)
		}
	})

	t.Run("guess for an already-revealed card", func(t *testing.T) {
		g := newTestGame(t)
		mustGiveClue(t, g, codenames.RedTeam, "red-things", 2)
		state, _, err := g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r0"})
		if err != nil {
			t.Fatalf("Move(guess r0): %v", err)
		}
		if state.NumGuessesLeft != 1 {
			t.Fatalf("NumGuessesLeft after 1st guess = %d, want 1", state.NumGuessesLeft)
		}

		if _, _, err := g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r0"}); err == nil {
			t.Fatal("Move(guess) for already-revealed card: got nil error, want error")
		}

		// The rejected re-guess shouldn't have consumed the remaining guess.
		state, _, err = g.Move(&Move{Action: ActionGuess, Team: codenames.RedTeam, Guess: "r1"})
		if err != nil {
			t.Fatalf("Move(guess r1) after rejected re-guess: %v", err)
		}
		if state.NumGuessesLeft != 0 {
			t.Fatalf("NumGuessesLeft after 2nd guess = %d, want 0", state.NumGuessesLeft)
		}
	})
}

func newTestGame(t *testing.T) *Game {
	t.Helper()
	g, err := New(testBoard(), codenames.RedTeam, testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func mustGiveClue(t *testing.T, g *Game, team codenames.Team, word string, count int) {
	t.Helper()
	if _, _, err := g.Move(&Move{
		Action:   ActionGiveClue,
		Team:     team,
		GiveClue: &codenames.Clue{Word: word, Count: count},
	}); err != nil {
		t.Fatalf("Move(clue %q %d): %v", word, count, err)
	}
}

// testConfig returns a Config with non-nil (but unused, since this test
// drives the game via Move() rather than Play()) player stubs, satisfying
// New()'s validation.
func testConfig() *Config {
	stubSM := stubSpymaster{}
	stubOp := stubOperative{}
	return &Config{
		RedSpymaster:  stubSM,
		BlueSpymaster: stubSM,
		RedOperative:  stubOp,
		BlueOperative: stubOp,
	}
}

// errUnexpectedCall is returned by the stub players below; these tests drive
// the game entirely through Move(), so neither should ever be called.
var errUnexpectedCall = errors.New("stub player was called; this test should only drive the game via Move()")

type stubSpymaster struct{}

func (stubSpymaster) GiveClue(*codenames.Board, codenames.Agent) (*codenames.Clue, error) {
	return nil, errUnexpectedCall
}

type stubOperative struct{}

func (stubOperative) Guess(*codenames.Board, *codenames.Clue) (string, error) {
	return "", errUnexpectedCall
}

// testBoard returns a valid 25-card board (9 red, 8 blue, 7 bystanders, 1
// assassin, matching what a Red-starting game requires) with predictable
// codenames.
func testBoard() *codenames.Board {
	var cards []codenames.Card
	add := func(prefix string, n int, agent codenames.Agent) {
		for i := 0; i < n; i++ {
			cards = append(cards, codenames.Card{
				Codename: prefix + strconv.Itoa(i),
				Agent:    agent,
			})
		}
	}
	add("r", 9, codenames.RedAgent)
	add("b", 8, codenames.BlueAgent)
	add("y", 7, codenames.Bystander)
	add("a", 1, codenames.Assassin)

	return &codenames.Board{Cards: cards}
}

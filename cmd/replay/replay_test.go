package main

import (
	"os"
	"strings"
	"testing"

	"github.com/bcspragu/Codenames/botproto"
)

// testdata/example.jsonl is the log of a real match between the example
// Python bots (seed 42).
func loadExample(t *testing.T) *replay {
	t.Helper()
	f, err := os.Open("testdata/example.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rp, err := loadReplay(f)
	if err != nil {
		t.Fatalf("loadReplay: %v", err)
	}
	return rp
}

func revealed(f frame) map[string]botproto.CardType {
	out := map[string]botproto.CardType{}
	for _, c := range f.board {
		if c.Revealed {
			out[c.Word] = c.Type
		}
	}
	return out
}

func TestLoadReplayBoardState(t *testing.T) {
	rp := loadExample(t)

	// One frame for match_start plus one per remaining line.
	if got, want := len(rp.frames), 65; got != want {
		t.Fatalf("got %d frames, want %d", got, want)
	}
	if rp.truncated {
		t.Error("complete log reported as truncated")
	}

	start := rp.frames[0]
	if len(revealed(start)) != 0 || start.left(botproto.TeamRed) != 8 || start.left(botproto.TeamBlue) != 9 {
		t.Errorf("bad starting frame: revealed %v, red %d left, blue %d left", revealed(start), start.left(botproto.TeamRed), start.left(botproto.TeamBlue))
	}

	// After line 2 (blue's guess "tail").
	f := rp.frames[2]
	if got := revealed(f); len(got) != 1 || got["tail"] != botproto.CardBlue {
		t.Errorf("after first guess, revealed = %v, want only tail=blue", got)
	}
	if f.turn != 1 || f.team != botproto.TeamBlue || f.clue == nil || f.clue.Word != "small" || f.left(botproto.TeamBlue) != 8 {
		t.Errorf("after first guess: turn %d, team %q, clue %+v, blue left %d", f.turn, f.team, f.clue, f.left(botproto.TeamBlue))
	}

	// After line 5 (red's guess "knife", a bystander).
	f = rp.frames[5]
	if got := revealed(f); len(got) != 2 || got["knife"] != botproto.CardBystander {
		t.Errorf("after second guess, revealed = %v, want tail and knife", got)
	}
	if f.turn != 2 || f.team != botproto.TeamRed || f.clue.Word != "money" || f.lastGuess.Word != "knife" {
		t.Errorf("after second guess: turn %d, team %q, clue %+v, last guess %+v", f.turn, f.team, f.clue, f.lastGuess)
	}

	// A new turn clears the previous turn's guess.
	if f := rp.frames[7]; f.turn != 3 || f.lastGuess != nil || f.clue.Word != "cold" {
		t.Errorf("new turn: turn %d, last guess %+v, clue %+v", f.turn, f.lastGuess, f.clue)
	}

	// Stepping back shows the earlier board unchanged.
	if len(revealed(rp.frames[2])) != 1 {
		t.Error("later frames changed an earlier frame's board")
	}

	last := rp.frames[len(rp.frames)-1]
	if last.result == nil || last.result.Winner != botproto.TeamBlue || last.left(botproto.TeamBlue) != 0 {
		t.Errorf("final frame: result %+v, blue left %d", last.result, last.left(botproto.TeamBlue))
	}
	if out := rp.render(len(rp.frames)-1, false, false); !strings.Contains(out, "Winner: beta (blue)") {
		t.Errorf("final render missing winner:\n%s", out)
	}
}

func TestLoadReplayBadLogs(t *testing.T) {
	data, err := os.ReadFile("testdata/example.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	start := lines[0]

	tests := []struct {
		name    string
		log     string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"not a move log", `{"hello": "world"}`, `line 1: expected the first line to be "match_start"`},
		{"cut off mid-line", start + "\n" + lines[1][:20], "line 2: invalid JSON"},
		{"guess not on board", start + "\n" + `{"type":"guess","turn":1,"team":"blue","word":"zzz","revealed":"blue"}`, `line 2: guess "zzz" is not on the board`},
		{"guess type mismatch", start + "\n" + `{"type":"guess","turn":1,"team":"blue","word":"tail","revealed":"red"}`, `starting board says it's "blue"`},
		{"unknown line type", start + "\n" + `{"type":"dance","turn":1,"team":"blue"}`, `unknown line type "dance"`},
		{"line after result", strings.Join(lines, "\n") + "\n" + lines[1], "after the result line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadReplay(strings.NewReader(tt.log))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}

	// A log that stops cleanly between lines still replays, marked truncated.
	rp, err := loadReplay(strings.NewReader(strings.Join(lines[:10], "\n")))
	if err != nil {
		t.Fatalf("truncated log: %v", err)
	}
	if !rp.truncated || !strings.Contains(rp.render(len(rp.frames)-1, false, false), "may have been interrupted") {
		t.Error("truncated log not reported as interrupted")
	}
}

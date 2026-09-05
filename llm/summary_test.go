package llm

import (
	"strings"
	"testing"

	"github.com/bcspragu/Codenames/codenames"
)

func TestParseWhyLine(t *testing.T) {
	targets := []string{"whale", "ship"}

	for _, tc := range []struct {
		name string
		line string
		want map[string]string
	}{
		{
			name: "hyphen separator",
			line: "whale - sea mammal; ship - sails the ocean",
			want: map[string]string{"whale": "sea mammal", "ship": "sails the ocean"},
		},
		{
			name: "em dash and colon separators",
			line: "whale — sea mammal; ship: sails the ocean",
			want: map[string]string{"whale": "sea mammal", "ship": "sails the ocean"},
		},
		{
			name: "case insensitive match onto canonical targets",
			line: "WHALE - sea mammal; Ship - sails the ocean",
			want: map[string]string{"whale": "sea mammal", "ship": "sails the ocean"},
		},
		{
			name: "hyphenated reason keeps its hyphen",
			line: "whale - a deep-sea mammal; ship - sails",
			want: map[string]string{"whale": "a deep-sea mammal", "ship": "sails"},
		},
		{
			name: "word that isn't a target is dropped",
			line: "whale - sea mammal; dinosaur - extinct",
			want: map[string]string{"whale": "sea mammal", "ship": ""},
		},
		{
			name: "missing line yields empty reasons, not missing targets",
			line: "",
			want: map[string]string{"whale": "", "ship": ""},
		},
		{
			name: "unparseable fragment is skipped, others survive",
			line: "whale; ship - sails the ocean",
			want: map[string]string{"whale": "", "ship": "sails the ocean"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWhyLine(tc.line, targets)
			if len(got) != len(targets) {
				t.Fatalf("parseWhyLine() returned %d entries, want %d (every target must always be present)", len(got), len(targets))
			}
			for _, tw := range got {
				if want := tc.want[tw.Word]; tw.Why != want {
					t.Errorf("parseWhyLine() why for %q = %q, want %q", tw.Word, tw.Why, want)
				}
			}
		})
	}
}

// A malformed WHY must never cost the model its clue — it's presentation
// only, and rejecting on it would burn a retry on a valid move.
func TestParseClueAcceptsBadWhy(t *testing.T) {
	board := []codenames.Card{{Codename: "whale"}, {Codename: "ship"}, {Codename: "shadow"}}
	reply := `ASSASSIN: clear, no relation to "shadow"
CLUE: ocean
TARGETS: whale, ship
WHY: this is not the expected format at all
NUMBER: 2`

	p, err := parseClue(reply, []string{"whale", "ship"}, board)
	if err != nil {
		t.Fatalf("parseClue() error = %v, want the clue accepted despite the malformed WHY", err)
	}
	if p.Clue.Word != "ocean" || p.Clue.Count != 2 {
		t.Errorf("parseClue() clue = %s %d, want ocean 2", p.Clue.Word, p.Clue.Count)
	}
	if len(p.Targets) != 2 {
		t.Fatalf("parseClue() targets = %d, want 2", len(p.Targets))
	}
}

func TestParseClueOmittedWhy(t *testing.T) {
	board := []codenames.Card{{Codename: "whale"}, {Codename: "ship"}, {Codename: "shadow"}}
	reply := `ASSASSIN: clear
CLUE: ocean
TARGETS: whale, ship
NUMBER: 2`

	p, err := parseClue(reply, []string{"whale", "ship"}, board)
	if err != nil {
		t.Fatalf("parseClue() error = %v, want nil", err)
	}
	for _, tw := range p.Targets {
		if tw.Why != "" {
			t.Errorf("parseClue() why for %q = %q, want empty", tw.Word, tw.Why)
		}
	}
}

func TestWrapText(t *testing.T) {
	got := wrapText("the quick brown fox jumps over the lazy dog", 12)
	for _, line := range got {
		if len([]rune(line)) > 12 {
			t.Errorf("wrapText() produced %q (%d runes), want <= 12", line, len([]rune(line)))
		}
	}
	if joined := strings.Join(got, " "); joined != "the quick brown fox jumps over the lazy dog" {
		t.Errorf("wrapText() lost or reordered words: %q", joined)
	}
	if got := wrapText("   ", 12); got != nil {
		t.Errorf("wrapText(whitespace) = %v, want nil", got)
	}
}

// The box is only legible if every line is bounded, so a pathologically long
// reason must be wrapped or truncated rather than blowing past the border.
func TestSummaryLinesAreBounded(t *testing.T) {
	long := strings.Repeat("supercalifragilistic ", 40)
	res := &GuessResult{
		Guess: "note", MustGuess: true, ClueNumber: 2,
		RiskiestBoardWord: "engine",
		Candidates: []Candidate{
			{Word: "note", Confidence: 0.4, RawConfidence: 0.6, LinkType: "multi_hop", Reasoning: long},
		},
	}
	out := operativeSummary("RED", &codenames.Clue{Word: "power", Count: 2}, res)
	for _, line := range strings.Split(out, "\n") {
		if n := len([]rune(line)); n > summaryWidth+4 {
			t.Errorf("summary line is %d runes, want <= %d: %q", n, summaryWidth+4, line)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate() = %q, want %q", got, "hello")
	}
	if got := truncate("hello world", 8); got != "hello w…" {
		t.Errorf("truncate() = %q, want %q", got, "hello w…")
	}
}

// Ollama reports a reasoning model's deliberation in one of two places
// depending on how thinking was enabled: inline <think> tags inside the
// content, or a separate "thinking" field on the message. Reading only the
// first makes a budget-exhausted reply (empty content, full thinking field)
// indistinguishable from a model that returned nothing, which defeats the
// retry that grows num_predict.
func TestThinkingFrom(t *testing.T) {
	for _, tc := range []struct {
		name         string
		raw          string
		msg          chatMessage
		wantReply    string
		wantThinking string
	}{
		{
			name:         "inline think tags",
			raw:          "<think>deliberating</think>CLUE: ocean",
			msg:          chatMessage{Content: "<think>deliberating</think>CLUE: ocean"},
			wantReply:    "CLUE: ocean",
			wantThinking: "deliberating",
		},
		{
			name:         "separate thinking field",
			raw:          "CLUE: ocean",
			msg:          chatMessage{Content: "CLUE: ocean", Thinking: "deliberating"},
			wantReply:    "CLUE: ocean",
			wantThinking: "deliberating",
		},
		{
			name:         "budget exhausted mid-thought, answer never reached",
			raw:          "",
			msg:          chatMessage{Content: "", Thinking: "still deliberating when the budget ran out"},
			wantReply:    "",
			wantThinking: "still deliberating when the budget ran out",
		},
		{
			name:         "no thinking at all",
			raw:          "CLUE: ocean",
			msg:          chatMessage{Content: "CLUE: ocean"},
			wantReply:    "CLUE: ocean",
			wantThinking: "",
		},
		{
			name:         "inline tags win over the field when both are present",
			raw:          "<think>inline</think>CLUE: ocean",
			msg:          chatMessage{Content: "<think>inline</think>CLUE: ocean", Thinking: "field"},
			wantReply:    "CLUE: ocean",
			wantThinking: "inline",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply, thinking := thinkingFrom(tc.raw, tc.msg)
			if reply != tc.wantReply {
				t.Errorf("thinkingFrom() reply = %q, want %q", reply, tc.wantReply)
			}
			if thinking != tc.wantThinking {
				t.Errorf("thinkingFrom() thinking = %q, want %q", thinking, tc.wantThinking)
			}
		})
	}
}

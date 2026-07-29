package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

var teamWords = []string{"Mug", "Mail", "Whale", "Ship"}

// testBoard holds the team words plus a few others, so clue-vs-board checks
// have something to collide with.
var testBoard = []codenames.Card{
	{Codename: "Mug", Agent: codenames.RedAgent},
	{Codename: "Mail", Agent: codenames.RedAgent},
	{Codename: "Whale", Agent: codenames.RedAgent},
	{Codename: "Ship", Agent: codenames.RedAgent},
	{Codename: "king", Agent: codenames.BlueAgent},
	{Codename: "police", Agent: codenames.BlueAgent},
	{Codename: "ice", Agent: codenames.Bystander},
	{Codename: "ninja", Agent: codenames.Assassin},
}

func TestParseClueResponse(t *testing.T) {
	tests := []struct {
		name      string
		reply     string
		wantWord  string
		wantCount int
	}{
		{
			name:      "count derived from targets",
			reply:     `{"clue":"lid","targets":["mug","mail"],"links":{"mug":"a mug has a lid","mail":"mail slots have lids"}}`,
			wantWord:  "lid",
			wantCount: 2,
		},
		{
			name:      "single target",
			reply:     `{"clue":"ocean","targets":["whale"],"links":{"whale":"whales live in the ocean"}}`,
			wantWord:  "ocean",
			wantCount: 1,
		},
		{
			name:      "fenced json is unwrapped",
			reply:     "```json\n{\"clue\":\"sea\",\"targets\":[\"whale\",\"ship\"],\"links\":{\"whale\":\"sea animal\",\"ship\":\"sails the sea\"}}\n```",
			wantWord:  "sea",
			wantCount: 2,
		},
		{
			name:      "clue is lowercased",
			reply:     `{"clue":"OCEAN","targets":["whale"],"links":{"whale":"ocean animal"}}`,
			wantWord:  "ocean",
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clue, reasoning, err := parseClueResponse(tc.reply, teamWords, testBoard)
			if err != nil {
				t.Fatalf("parseClueResponse() error = %v, want nil", err)
			}
			if clue.Word != tc.wantWord {
				t.Errorf("clue.Word = %q, want %q", clue.Word, tc.wantWord)
			}
			if clue.Count != tc.wantCount {
				t.Errorf("clue.Count = %d, want %d", clue.Count, tc.wantCount)
			}
			if reasoning == "" {
				t.Error("reasoning was empty, want per-target justifications")
			}
		})
	}
}

// TestCountIgnoresModelStatedNumber is the point of the schema: even when the
// model volunteers a number, the count comes from the target list.
func TestCountIgnoresModelStatedNumber(t *testing.T) {
	reply := `{"clue":"lid","count":5,"number":5,"targets":["mug","mail"],"links":{"mug":"has a lid","mail":"mail slot lid"}}`

	clue, _, err := parseClueResponse(reply, teamWords, testBoard)
	if err != nil {
		t.Fatalf("parseClueResponse() error = %v, want nil", err)
	}
	if clue.Count != 2 {
		t.Errorf("clue.Count = %d, want 2 (len(targets)); a model-stated count leaked through", clue.Count)
	}
}

func TestParseClueResponseRejects(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{
			name:  "links missing an entry for a target",
			reply: `{"clue":"lid","targets":["mug","mail"],"links":{"mug":"a mug has a lid"}}`,
		},
		{
			name:  "links has an entry not in targets",
			reply: `{"clue":"lid","targets":["mug"],"links":{"mug":"has a lid","mail":"mail slot"}}`,
		},
		{
			name:  "links same size but different key set",
			reply: `{"clue":"lid","targets":["mug","mail"],"links":{"mug":"has a lid","whale":"blowhole"}}`,
		},
		{
			name:  "links entry is blank",
			reply: `{"clue":"lid","targets":["mug","mail"],"links":{"mug":"has a lid","mail":"   "}}`,
		},
		{
			name:  "links omitted entirely",
			reply: `{"clue":"lid","targets":["mug","mail"]}`,
		},
		{
			name:  "duplicate target would inflate the count",
			reply: `{"clue":"lid","targets":["mug","mug"],"links":{"mug":"has a lid"}}`,
		},
		{
			name:  "target is not one of our words",
			reply: `{"clue":"lid","targets":["mug","kettle"],"links":{"mug":"has a lid","kettle":"has a lid"}}`,
		},
		{
			name:  "empty targets",
			reply: `{"clue":"lid","targets":[],"links":{}}`,
		},
		{
			name:  "empty clue",
			reply: `{"clue":"","targets":["mug"],"links":{"mug":"has a lid"}}`,
		},
		{
			name:  "multi-word clue",
			reply: `{"clue":"coffee mug","targets":["mug"],"links":{"mug":"a mug"}}`,
		},
		{
			name:  "hyphenated clue",
			reply: `{"clue":"sea-going","targets":["ship"],"links":{"ship":"goes to sea"}}`,
		},
		{
			name:  "not json at all",
			reply: "OCEAN 3\nREASON: whale, ship and wave are in the ocean",
		},
		{
			name:  "malformed json",
			reply: `{"clue":"lid","targets":["mug",}`,
		},
		{
			// The real incident from logs/ai_reasoning.jsonl: targets and
			// links were perfectly well-formed, but the clue was a board word.
			name:  "clue is a board word",
			reply: `{"clue":"king","targets":["mug","mail"],"links":{"mug":"a king drinks from a mug","mail":"the king receives mail"}}`,
		},
		{
			name:  "clue contains a board word",
			reply: `{"clue":"kingdom","targets":["mug"],"links":{"mug":"royal mug"}}`,
		},
		{
			name:  "clue is a board word pluralized",
			reply: `{"clue":"kings","targets":["mug"],"links":{"mug":"kings drink from mugs"}}`,
		},
		{
			name:  "clue is a board word in a different case",
			reply: `{"clue":"KING","targets":["mug"],"links":{"mug":"royal mug"}}`,
		},
		{
			name:  "clue is one of our own target words",
			reply: `{"clue":"whale","targets":["ship"],"links":{"ship":"whales and ships share the sea"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clue, _, err := parseClueResponse(tc.reply, teamWords, testBoard)
			if err == nil {
				t.Fatalf("parseClueResponse() = %+v, want hard reject", clue)
			}
		})
	}
}

// TestTargetsMatchBoardCasing checks that targets come back in the board's own
// spelling regardless of how the model cased them.
func TestTargetsMatchBoardCasing(t *testing.T) {
	reply := `{"clue":"lid","targets":["MUG","mAiL"],"links":{"mug":"has a lid","MAIL":"mail slot"}}`

	clue, reasoning, err := parseClueResponse(reply, teamWords, testBoard)
	if err != nil {
		t.Fatalf("parseClueResponse() error = %v, want nil", err)
	}
	if clue.Count != 2 {
		t.Errorf("clue.Count = %d, want 2", clue.Count)
	}
	for _, want := range []string{"Mug", "Mail"} {
		if !strings.Contains(reasoning, want+":") {
			t.Errorf("reasoning %q missing board-cased target %q", reasoning, want)
		}
	}
}

// fakeOllama serves canned replies in order, one per request.
func fakeOllama(t *testing.T, replies ...string) (*AI, func() int) {
	t.Helper()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := calls
		calls++
		if i >= len(replies) {
			t.Errorf("ollama called %d times, only %d replies canned", calls, len(replies))
			i = len(replies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(chatResponse{
			Message: chatMessage{Role: "assistant", Content: replies[i]},
		}); err != nil {
			t.Errorf("encode canned reply: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	return New(srv.URL, "test-model", 10*time.Second), func() int { return calls }
}

func retryTestBoard() *codenames.Board {
	return &codenames.Board{Cards: testBoard}
}

// TestGiveClueRetriesPastIllegalClue exercises the self-healing loop: the model
// first names a board word, and the corrective turn gets it to a legal clue
// without the caller ever seeing the illegal one.
func TestGiveClueRetriesPastIllegalClue(t *testing.T) {
	ai, calls := fakeOllama(t,
		`{"clue":"king","targets":["mug","mail"],"links":{"mug":"royal mug","mail":"royal mail"}}`,
		`{"clue":"royal","targets":["mug","mail"],"links":{"mug":"a royal mug","mail":"royal mail"}}`,
	)

	clue, reasoning, err := ai.GiveClueWithReasoning(retryTestBoard(), codenames.RedAgent)
	if err != nil {
		t.Fatalf("GiveClueWithReasoning() error = %v, want nil", err)
	}
	if clue.Word != "royal" {
		t.Errorf("clue.Word = %q, want %q", clue.Word, "royal")
	}
	if clue.Count != 2 {
		t.Errorf("clue.Count = %d, want 2", clue.Count)
	}
	if reasoning == "" {
		t.Error("reasoning was empty")
	}
	if got := calls(); got != 2 {
		t.Errorf("ollama calls = %d, want 2 (one rejection, one recovery)", got)
	}
}

// TestGiveClueGivesUpRatherThanReturningIllegalClue: if the model never
// produces a legal clue, we must surface an error, never a board word.
func TestGiveClueGivesUpRatherThanReturningIllegalClue(t *testing.T) {
	illegal := `{"clue":"king","targets":["mug"],"links":{"mug":"royal mug"}}`
	ai, calls := fakeOllama(t, illegal, illegal, illegal)

	clue, _, err := ai.GiveClueWithReasoning(retryTestBoard(), codenames.RedAgent)
	if err == nil {
		t.Fatalf("GiveClueWithReasoning() = %+v, want error after exhausting retries", clue)
	}
	if clue != nil {
		t.Errorf("clue = %+v, want nil", clue)
	}
	if got := calls(); got != 3 {
		t.Errorf("ollama calls = %d, want 3", got)
	}
}

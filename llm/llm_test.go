package llm

import (
	"encoding/json"
	"fmt"
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

// validClueReply builds a schema-conformant spymaster reply: an ASSASSIN
// line and CLUE/TARGETS/NUMBER lines derived from clue and targets.
func validClueReply(clue string, targets []string) string {
	targetsLine := strings.Join(targets, ", ")
	return fmt.Sprintf(`ASSASSIN: clear

CLUE: %s
TARGETS: %s
NUMBER: %d`, clue, targetsLine, len(targets))
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
			reply:     validClueReply("lid", []string{"mug", "mail"}),
			wantWord:  "lid",
			wantCount: 2,
		},
		{
			name:      "single target",
			reply:     validClueReply("ocean", []string{"whale"}),
			wantWord:  "ocean",
			wantCount: 1,
		},
		{
			name:      "clue is lowercased",
			reply:     validClueReply("OCEAN", []string{"whale"}),
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
				t.Error("reasoning was empty, want assassin-note/target justification")
			}
		})
	}
}

// TestReasoningIncludesSafetyChecks ensures the assassin safety note the
// model was forced to write out actually makes it into the logged
// reasoning, not just into a validation gate that then discards it.
func TestReasoningIncludesSafetyChecks(t *testing.T) {
	reply := `ASSASSIN: too close to assassin "ninja", avoided by picking lid instead

CLUE: lid
TARGETS: mug
NUMBER: 1`

	_, reasoning, err := parseClueResponse(reply, teamWords, testBoard)
	if err != nil {
		t.Fatalf("parseClueResponse() error = %v, want nil", err)
	}
	if !strings.Contains(reasoning, "ninja") {
		t.Errorf("reasoning %q missing candidate danger notes content", reasoning)
	}
}

func TestParseClueResponseRejects(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{
			name:  "not schema at all",
			reply: "OCEAN 3\nREASON: whale, ship and wave are in the ocean",
		},
		{
			name: "assassin line missing",
			reply: `CLUE: lid
TARGETS: mug, mail
NUMBER: 2`,
		},
		{
			name: "assassin line empty",
			reply: `ASSASSIN:

CLUE: lid
TARGETS: mug
NUMBER: 1`,
		},
		{
			name: "duplicate target would inflate the count",
			reply: `ASSASSIN: clear

CLUE: lid
TARGETS: mug, mug
NUMBER: 2`,
		},
		{
			name:  "target is not one of our words",
			reply: validClueReply("lid", []string{"mug", "kettle"}),
		},
		{
			name: "empty targets",
			reply: `ASSASSIN: clear

CLUE: lid
TARGETS:
NUMBER: 0`,
		},
		{
			name:  "empty clue",
			reply: validClueReply("", []string{"mug"}),
		},
		{
			name:  "multi-word clue",
			reply: validClueReply("coffee mug", []string{"mug"}),
		},
		{
			name:  "hyphenated clue",
			reply: validClueReply("sea-going", []string{"ship"}),
		},
		{
			// The real incident from logs/ai_reasoning.jsonl: targets were
			// perfectly well-formed, but the clue was a board word.
			name:  "clue is a board word",
			reply: validClueReply("king", []string{"mug", "mail"}),
		},
		{
			name:  "clue contains a board word",
			reply: validClueReply("kingdom", []string{"mug"}),
		},
		{
			name:  "clue is a board word pluralized",
			reply: validClueReply("kings", []string{"mug"}),
		},
		{
			name:  "clue is a board word in a different case",
			reply: validClueReply("KING", []string{"mug"}),
		},
		{
			name:  "clue is one of our own target words",
			reply: validClueReply("whale", []string{"ship"}),
		},
		{
			name: "NUMBER does not match TARGETS count",
			reply: `ASSASSIN: clear

CLUE: lid
TARGETS: mug, mail
NUMBER: 5`,
		},
		{
			name: "NUMBER is not an integer",
			reply: `ASSASSIN: clear

CLUE: lid
TARGETS: mug
NUMBER: many`,
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
	reply := validClueReply("lid", []string{"MUG", "mAiL"})

	clue, reasoning, err := parseClueResponse(reply, teamWords, testBoard)
	if err != nil {
		t.Fatalf("parseClueResponse() error = %v, want nil", err)
	}
	if clue.Count != 2 {
		t.Errorf("clue.Count = %d, want 2", clue.Count)
	}
	for _, want := range []string{"Mug", "Mail"} {
		if !strings.Contains(reasoning, want) {
			t.Errorf("reasoning %q missing board-cased target %q", reasoning, want)
		}
	}
}

func TestSplitThinking(t *testing.T) {
	tests := []struct {
		name         string
		reply        string
		wantClean    string
		wantThinking string
	}{
		{
			name:         "no think block is untouched",
			reply:        "OCEAN\nREASON: whale lives there",
			wantClean:    "OCEAN\nREASON: whale lives there",
			wantThinking: "",
		},
		{
			name:         "leading think block is removed and captured",
			reply:        "<think>hmm, whale seems right because...</think>\nOCEAN\nREASON: whale lives there",
			wantClean:    "OCEAN\nREASON: whale lives there",
			wantThinking: "hmm, whale seems right because...",
		},
		{
			name:         "think block containing braces doesn't break extraction",
			reply:        `<think>the reply should look like {"clue": "x"}</think>CLUE: ocean`,
			wantClean:    `CLUE: ocean`,
			wantThinking: `the reply should look like {"clue": "x"}`,
		},
		{
			// Tag casing is exact, not folded: the model doesn't choose it,
			// Ollama's chat template does, so a different case never appears
			// in practice. A mismatched case is left untouched rather than
			// silently misparsed.
			name:         "wrong-case tags are left untouched",
			reply:        "<Think>reasoning here</Think>\nOCEAN",
			wantClean:    "<Think>reasoning here</Think>\nOCEAN",
			wantThinking: "",
		},
		{
			name:         "unterminated think block drops everything from the tag onward, keeping it as thinking",
			reply:        "<think>still reasoning when the budget ran out",
			wantClean:    "",
			wantThinking: "still reasoning when the budget ran out",
		},
		{
			// Observed from qwen3 via Ollama with the top-level "think" field
			// set to false: it still emits a bare "</think>" with no matching
			// opening tag, right before the real answer.
			name:         "bare closing tag with no opening tag is still split on",
			reply:        "Let me reason about this without an opening tag.</think>\nShip\nREASON: it's a boat",
			wantClean:    "Ship\nREASON: it's a boat",
			wantThinking: "Let me reason about this without an opening tag.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clean, thinking := splitThinking(tc.reply)
			if clean != tc.wantClean {
				t.Errorf("splitThinking(%q) clean = %q, want %q", tc.reply, clean, tc.wantClean)
			}
			if thinking != tc.wantThinking {
				t.Errorf("splitThinking(%q) thinking = %q, want %q", tc.reply, thinking, tc.wantThinking)
			}
		})
	}
}

// TestGiveClueStripsThinkingBlock is the end-to-end version of
// TestSplitThinking: a reasoning model's chain-of-thought preamble must not
// leak into clue parsing, but should still surface in the returned reasoning
// so it reaches ai_reasoning.jsonl / the admin UI.
func TestGiveClueStripsThinkingBlock(t *testing.T) {
	ai, calls := fakeOllama(t,
		`<think>I should connect mug and mail, maybe "royal" works since {both relate to the crown}</think>`+validClueReply("royal", []string{"mug", "mail"}),
	)

	clue, reasoning, err := ai.GiveClueWithReasoning(retryTestBoard(), codenames.RedAgent)
	if err != nil {
		t.Fatalf("GiveClueWithReasoning() error = %v, want nil", err)
	}
	if clue.Word != "royal" || clue.Count != 2 {
		t.Errorf("clue = %+v, want royal/2", clue)
	}
	if !strings.Contains(reasoning, `maybe "royal" works since`) {
		t.Errorf("reasoning %q missing the model's chain-of-thought", reasoning)
	}
	if !strings.Contains(reasoning, "Mug") {
		t.Errorf("reasoning %q missing the derived target justification", reasoning)
	}
	if got := calls(); got != 1 {
		t.Errorf("ollama calls = %d, want 1", got)
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

	return New(srv.URL, "test-model", 10*time.Second, DefaultMaxTokens), func() int { return calls }
}

func retryTestBoard() *codenames.Board {
	return &codenames.Board{Cards: testBoard}
}

// TestGiveClueRetriesPastIllegalClue exercises the self-healing loop: the model
// first names a board word, and the corrective turn gets it to a legal clue
// without the caller ever seeing the illegal one.
func TestGiveClueRetriesPastIllegalClue(t *testing.T) {
	ai, calls := fakeOllama(t,
		validClueReply("king", []string{"mug", "mail"}),
		validClueReply("royal", []string{"mug", "mail"}),
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
	illegal := validClueReply("king", []string{"mug"})
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

var boardWords = []string{"Pot", "Spoon", "Chef", "Fire"}

func TestParseCandidateResponse(t *testing.T) {
	reply := `{
  "candidates": [
    {"word": "Pot", "confidence": 0.95, "reasoning": "direct", "link_type": "direct"},
    {"word": "Fire", "confidence": 0.35, "reasoning": "idiom", "link_type": "idiom"}
  ],
  "riskiest_board_word": "Fire",
  "top_candidate_is_riskiest": false
}`
	resp, err := parseCandidateResponse(reply, boardWords)
	if err != nil {
		t.Fatalf("parseCandidateResponse() error = %v, want nil", err)
	}
	if len(resp.Candidates) != 2 {
		t.Fatalf("len(Candidates) = %d, want 2", len(resp.Candidates))
	}
	if resp.Candidates[0].Word != "Pot" || resp.Candidates[0].Confidence != 0.95 {
		t.Errorf("Candidates[0] = %+v, want Pot/0.95", resp.Candidates[0])
	}
	if resp.RiskiestBoardWord != "Fire" {
		t.Errorf("RiskiestBoardWord = %q, want Fire", resp.RiskiestBoardWord)
	}
}

func TestParseCandidateResponseStripsCodeFences(t *testing.T) {
	reply := "```json\n" + `{"candidates": [{"word": "Pot", "confidence": 0.9, "reasoning": "x", "link_type": "direct"}], "riskiest_board_word": "Fire", "top_candidate_is_riskiest": false}` + "\n```"
	resp, err := parseCandidateResponse(reply, boardWords)
	if err != nil {
		t.Fatalf("parseCandidateResponse() error = %v, want nil", err)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Word != "Pot" {
		t.Errorf("Candidates = %+v, want single Pot candidate", resp.Candidates)
	}
}

func TestParseCandidateResponseExtractsJSONFromNarration(t *testing.T) {
	reply := `Sure, here's my analysis. I think Pot is the best fit.

{"candidates": [{"word": "Pot", "confidence": 0.9, "reasoning": "x", "link_type": "direct"}], "riskiest_board_word": "Fire", "top_candidate_is_riskiest": false}

Hope that helps!`
	resp, err := parseCandidateResponse(reply, boardWords)
	if err != nil {
		t.Fatalf("parseCandidateResponse() error = %v, want nil", err)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Word != "Pot" {
		t.Errorf("Candidates = %+v, want single Pot candidate", resp.Candidates)
	}
}

func TestParseCandidateResponseCanonicalizesWordCasing(t *testing.T) {
	reply := `{"candidates": [{"word": "POT", "confidence": 0.9, "reasoning": "x", "link_type": "direct"}], "riskiest_board_word": "fire", "top_candidate_is_riskiest": false}`
	resp, err := parseCandidateResponse(reply, boardWords)
	if err != nil {
		t.Fatalf("parseCandidateResponse() error = %v, want nil", err)
	}
	if resp.Candidates[0].Word != "Pot" {
		t.Errorf("Candidates[0].Word = %q, want board-cased %q", resp.Candidates[0].Word, "Pot")
	}
	if resp.RiskiestBoardWord != "Fire" {
		t.Errorf("RiskiestBoardWord = %q, want board-cased %q", resp.RiskiestBoardWord, "Fire")
	}
}

func TestParseCandidateResponseDropsHallucinatedWordsKeepsValidOnes(t *testing.T) {
	reply := `{"candidates": [
    {"word": "Teapot", "confidence": 0.9, "reasoning": "not on board", "link_type": "direct"},
    {"word": "Pot", "confidence": 0.6, "reasoning": "on board", "link_type": "direct"}
  ], "riskiest_board_word": "Fire", "top_candidate_is_riskiest": false}`
	resp, err := parseCandidateResponse(reply, boardWords)
	if err != nil {
		t.Fatalf("parseCandidateResponse() error = %v, want nil (should drop hallucinated candidate, keep valid one)", err)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Word != "Pot" {
		t.Errorf("Candidates = %+v, want only the valid Pot candidate", resp.Candidates)
	}
}

func TestParseCandidateResponseRejects(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{"not JSON at all", "I think it's Pot."},
		{"empty candidates", `{"candidates": [], "riskiest_board_word": "Fire", "top_candidate_is_riskiest": false}`},
		{"all candidates hallucinated", `{"candidates": [{"word": "Teapot", "confidence": 0.9, "reasoning": "x", "link_type": "direct"}], "riskiest_board_word": "Fire", "top_candidate_is_riskiest": false}`},
		{"malformed JSON", `{"candidates": [{"word": "Pot", "confidence": 0.9,]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCandidateResponse(tc.reply, boardWords); err == nil {
				t.Fatalf("parseCandidateResponse(%q) = nil error, want hard reject", tc.reply)
			}
		})
	}
}

func TestParseCandidateResponseClampsOutOfRangeConfidence(t *testing.T) {
	reply := `{"candidates": [{"word": "Pot", "confidence": 1.5, "reasoning": "x", "link_type": "direct"}], "riskiest_board_word": "Fire", "top_candidate_is_riskiest": false}`
	resp, err := parseCandidateResponse(reply, boardWords)
	if err != nil {
		t.Fatalf("parseCandidateResponse() error = %v, want nil", err)
	}
	if resp.Candidates[0].Confidence != 1.0 {
		t.Errorf("Confidence = %v, want clamped to 1.0", resp.Candidates[0].Confidence)
	}
}

func TestTopCandidatePicksHighestConfidenceRegardlessOfOrder(t *testing.T) {
	candidates := []Candidate{
		{Word: "Fire", Confidence: 0.35},
		{Word: "Pot", Confidence: 0.95},
		{Word: "Chef", Confidence: 0.5},
	}
	top := topCandidate(candidates)
	if top.Word != "Pot" {
		t.Errorf("topCandidate() = %q, want Pot (highest confidence, even though listed second)", top.Word)
	}
}

func TestDecideGuess(t *testing.T) {
	cfg := DefaultGuessDecisionConfig // Mandated 0.55, Bonus 0.80, RiskiestPenalty 0.15

	tests := []struct {
		name            string
		mustGuess       bool
		guessesThisTurn int
		clueNumber      int
		top             Candidate
		topIsRiskiest   bool
		wantGuess       string
		wantThreshold   float64
	}{
		{
			name:      "mustGuess always takes top candidate regardless of confidence",
			mustGuess: true,
			top:       Candidate{Word: "Pot", Confidence: 0.05},
			wantGuess: "Pot", wantThreshold: 0,
		},
		{
			name:            "optional guess above mandated threshold guesses",
			guessesThisTurn: 0, clueNumber: 2,
			top:       Candidate{Word: "Pot", Confidence: 0.6},
			wantGuess: "Pot", wantThreshold: cfg.MandatedThreshold,
		},
		{
			name:            "optional guess below mandated threshold passes",
			guessesThisTurn: 0, clueNumber: 2,
			top:       Candidate{Word: "Fire", Confidence: 0.35},
			wantGuess: codenames.PassGuess, wantThreshold: cfg.MandatedThreshold,
		},
		{
			name:            "bonus guess (already met clue count) needs the higher bonus threshold",
			guessesThisTurn: 2, clueNumber: 2,
			top:       Candidate{Word: "Pot", Confidence: 0.6},
			wantGuess: codenames.PassGuess, wantThreshold: cfg.BonusThreshold,
		},
		{
			name:            "bonus guess clearing the bonus threshold still guesses",
			guessesThisTurn: 2, clueNumber: 2,
			top:       Candidate{Word: "Pot", Confidence: 0.95},
			wantGuess: "Pot", wantThreshold: cfg.BonusThreshold,
		},
		{
			name:            "self-flagged riskiest candidate raises the bar past what would otherwise clear it",
			guessesThisTurn: 0, clueNumber: 2,
			top: Candidate{Word: "Fire", Confidence: 0.6}, topIsRiskiest: true,
			wantGuess: codenames.PassGuess, wantThreshold: cfg.MandatedThreshold + cfg.RiskiestWordPenalty,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			guess, threshold := decideGuess(cfg, tc.mustGuess, tc.guessesThisTurn, tc.clueNumber, tc.top, tc.topIsRiskiest)
			if guess != tc.wantGuess {
				t.Errorf("decideGuess() guess = %q, want %q", guess, tc.wantGuess)
			}
			if threshold != tc.wantThreshold {
				t.Errorf("decideGuess() threshold = %v, want %v", threshold, tc.wantThreshold)
			}
		})
	}
}

// TestDecideGuessBoundaryIsCautious checks that a candidate sitting exactly
// on the threshold passes rather than guesses: confidence <= threshold, not <.
func TestDecideGuessBoundaryIsCautious(t *testing.T) {
	cfg := DefaultGuessDecisionConfig
	guess, threshold := decideGuess(cfg, false /* mustGuess */, 0, 2, Candidate{Word: "Pot", Confidence: cfg.MandatedThreshold}, false)
	if guess != codenames.PassGuess {
		t.Errorf("decideGuess() with confidence exactly at threshold = %q, want PASS (ties go to caution)", guess)
	}
	if threshold != cfg.MandatedThreshold {
		t.Errorf("decideGuess() threshold = %v, want %v", threshold, cfg.MandatedThreshold)
	}
}

func TestApplyLinkTypeCaps(t *testing.T) {
	cfg := DefaultGuessDecisionConfig // direct 1.00, category 0.75, idiom 0.35, multi_hop 0.40, unknown 0.35

	tests := []struct {
		name           string
		candidates     []Candidate
		wantConfidence []float64
		wantCapApplied bool
		wantUnknown    int
	}{
		{
			name:           "category caps to 0.75",
			candidates:     []Candidate{{Word: "Detention", Confidence: 0.95, LinkType: "category"}},
			wantConfidence: []float64{0.75},
			wantCapApplied: true,
		},
		{
			name:           "idiom caps to 0.35",
			candidates:     []Candidate{{Word: "Fire", Confidence: 0.90, LinkType: "idiom"}},
			wantConfidence: []float64{0.35},
			wantCapApplied: true,
		},
		{
			name:           "multi_hop caps to 0.40",
			candidates:     []Candidate{{Word: "Menu", Confidence: 0.99, LinkType: "multi_hop"}},
			wantConfidence: []float64{0.40},
			wantCapApplied: true,
		},
		{
			name:           "direct is untouched",
			candidates:     []Candidate{{Word: "Pot", Confidence: 0.95, LinkType: "direct"}},
			wantConfidence: []float64{0.95},
			wantCapApplied: false,
		},
		{
			name:           "unknown link_type gets the conservative 0.35 cap",
			candidates:     []Candidate{{Word: "Chef", Confidence: 0.80, LinkType: "synonym"}},
			wantConfidence: []float64{0.35},
			wantCapApplied: true,
			wantUnknown:    1,
		},
		{
			name:           "empty link_type gets the conservative 0.35 cap",
			candidates:     []Candidate{{Word: "Chef", Confidence: 0.80, LinkType: ""}},
			wantConfidence: []float64{0.35},
			wantCapApplied: true,
			wantUnknown:    1,
		},
		{
			name:           "link_type matched case-insensitively",
			candidates:     []Candidate{{Word: "Fire", Confidence: 0.90, LinkType: "IDIOM"}},
			wantConfidence: []float64{0.35},
			wantCapApplied: true,
		},
		{
			name: "a candidate already under its cap is left alone",
			candidates: []Candidate{
				{Word: "Fire", Confidence: 0.20, LinkType: "idiom"},
			},
			wantConfidence: []float64{0.20},
			wantCapApplied: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capApplied, unknown := applyLinkTypeCaps(cfg, tc.candidates)
			for i, want := range tc.wantConfidence {
				if tc.candidates[i].Confidence != want {
					t.Errorf("candidate[%d].Confidence = %v, want %v", i, tc.candidates[i].Confidence, want)
				}
			}
			if capApplied != tc.wantCapApplied {
				t.Errorf("capApplied = %v, want %v", capApplied, tc.wantCapApplied)
			}
			if unknown != tc.wantUnknown {
				t.Errorf("unknownLinkTypes = %d, want %d", unknown, tc.wantUnknown)
			}
		})
	}
}

// TestDetentionRegression reproduces the exact failure from the school-board
// live run: clue "office 2", bonus guess (guessesThisTurn == clueNumber, so
// BonusThreshold applies), and the model rated the assassin word "Detention"
// at confidence 0.95 with link_type "category" — not even one of the
// link types (idiom/multi_hop) the prompt explicitly warns it to cap. Because
// 0.95 cleared the 0.80 bonus threshold, the old code guessed and hit the
// assassin. The category cap (0.75) must now catch this before the threshold
// comparison ever runs, independent of whether the riskiest-word self-check
// correctly flagged the word (it didn't, in the live run — Book was
// self-flagged as riskiest instead of Detention).
func TestDetentionRegression(t *testing.T) {
	cfg := DefaultGuessDecisionConfig
	candidates := []Candidate{
		{Word: "Detention", Confidence: 0.95, LinkType: "category", Reasoning: "School office directly handles detention assignments and management."},
		{Word: "Book", Confidence: 0.70, LinkType: "category"},
		{Word: "Exam", Confidence: 0.30, LinkType: "multi_hop"},
	}

	capApplied, _ := applyLinkTypeCaps(cfg, candidates)
	if !capApplied {
		t.Fatal("applyLinkTypeCaps() capApplied = false, want true (Detention's 0.95 category confidence should have been capped)")
	}
	if candidates[0].Confidence != 0.75 {
		t.Fatalf("Detention's confidence after capping = %v, want 0.75", candidates[0].Confidence)
	}

	top := topCandidate(candidates)
	if top.Word != "Detention" {
		t.Fatalf("topCandidate() = %q, want Detention (0.75 is still the highest of the three)", top.Word)
	}

	// riskiestBoardWord/topIsRiskiest deliberately mirror the live run, where
	// the self-check named the wrong word (Book) — the cap alone must be
	// sufficient to produce a pass, without relying on that check being right.
	guess, threshold := decideGuess(cfg, false /* mustGuess */, 2 /* guessesThisTurn */, 2 /* clueNumber */, top, false /* topIsRiskiest */)
	if guess != codenames.PassGuess {
		t.Errorf("decideGuess() = %q, want PASS — capped confidence 0.75 must not clear the 0.80 bonus threshold", guess)
	}
	if threshold != cfg.BonusThreshold {
		t.Errorf("threshold = %v, want %v", threshold, cfg.BonusThreshold)
	}
}

// TestApplyLinkTypeCapsReordersTopCandidate checks that capping happens
// before ranking: a category candidate that starts on top can be capped
// below an uncapped direct candidate, and topCandidate must then return the
// direct one.
func TestApplyLinkTypeCapsReordersTopCandidate(t *testing.T) {
	cfg := DefaultGuessDecisionConfig
	candidates := []Candidate{
		{Word: "Chef", Confidence: 0.95, LinkType: "category"}, // caps to 0.75
		{Word: "Pot", Confidence: 0.80, LinkType: "direct"},    // uncapped
	}

	applyLinkTypeCaps(cfg, candidates)

	top := topCandidate(candidates)
	if top.Word != "Pot" {
		t.Fatalf("topCandidate() after capping = %q, want Pot (0.80 uncapped beats Chef's capped 0.75)", top.Word)
	}
	if top.Confidence != 0.80 {
		t.Errorf("top.Confidence = %v, want 0.80", top.Confidence)
	}
}

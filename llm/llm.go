// Package llm provides Codenames Spymaster and Operative implementations
// backed by a local LLM via the Ollama API.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

const DefaultTimeout = 3 * time.Minute
const DefaultMaxTokens = 3000
const DefaultGuessMaxTokens = 3000
const DefaultTemperature = 0.6

// AI implements codenames.Spymaster and codenames.Operative using a local
// Ollama model.
type AI struct {
	endpoint    string // e.g. "http://localhost:11434"
	model       string // e.g. "llama3"
	timeout     time.Duration
	maxTokens   int
	temperature float64
	seed *int
	think *bool
	guessConfig GuessDecisionConfig
	parseErrorCount atomic.Int64
	unknownLinkTypeCount atomic.Int64
}

// Option customizes an AI beyond the required New arguments.
type Option func(*AI)

// WithTemperature overrides DefaultTemperature.
func WithTemperature(temperature float64) Option {
	return func(ai *AI) { ai.temperature = temperature }
}

// WithSeed fixes Ollama's sampling seed so identical prompts yield identical replies across runs.
func WithSeed(seed int) Option {
	return func(ai *AI) { ai.seed = &seed }
}

func WithThink(think bool) Option {
	return func(ai *AI) { ai.think = &think }
}

func WithGuessDecisionConfig(cfg GuessDecisionConfig) Option {
	return func(ai *AI) { ai.guessConfig = cfg }
}

func (ai *AI) deliberationInstructions() string {
	if ai.think != nil && !*ai.think {
		return `Thinking is disabled for this call, so you have no private scratch space — anything you write is your final answer. Do NOT narrate your deliberation, list candidates, or explain your reasoning in prose. Weigh your candidates against the assassin/opponent words/bystanders silently, commit to one, and reply with ONLY the schema below — nothing before it, nothing after it.`
	}
	return `Inside your thinking, weigh your candidates against the assassin/opponent words/bystanders as you go, and commit to a verdict immediately — once you've ruled a candidate out as unsafe or weak, never come back to reconsider it. Keep your thinking short and decisive; looping back over candidates you've already rejected is the main way you run out of budget. If you notice yourself still undecided, immediately stop deliberating and fall back to your single safest, highest-confidence one-target candidate — a safe 1-word clue that ships beats an ambitious one that times out.

After you're done thinking, output nothing but the schema below, in this exact order — no summary, no repetition of your thinking outside it:`
}

func (ai *AI) guessFormatInstructions() string {
	if ai.think != nil && !*ai.think {
		return `Thinking is disabled for this call, so you have no private scratch space — anything you write is your final answer. Do NOT narrate your reasoning, list your deliberation, or explain yourself outside the JSON object. Put each candidate's one-sentence reasoning in that candidate's "reasoning" field and nowhere else.`
	}
	return `Do your reasoning inside your thinking. Once you're done thinking, output nothing but the JSON object below — no summary, no explanation, no repetition of your thinking outside it.`
}

func New(endpoint, model string, timeout time.Duration, maxTokens int, opts ...Option) *AI {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	ai := &AI{endpoint: endpoint, model: model, timeout: timeout, maxTokens: maxTokens, temperature: DefaultTemperature, guessConfig: DefaultGuessDecisionConfig}
	for _, opt := range opts {
		opt(ai)
	}
	return ai
}

// Ollama chat API types

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format string `json:"format,omitempty"`
	KeepAlive string `json:"keep_alive,omitempty"`
	Think *bool `json:"think,omitempty"`
	Options chatReqOptions `json:"options"`
}

type chatReqOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature"`
	Seed        *int    `json:"seed,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message         chatMessage `json:"message"`
	TotalDuration   int64       `json:"total_duration"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
}

func (ai *AI) chat(ctx context.Context, messages []chatMessage, format string, numPredict int) (string, chatResponse, error) {
	body, err := json.Marshal(chatRequest{
		Model:     ai.model,
		Messages:  messages,
		Stream:    false,
		Format:    format,
		KeepAlive: "10m",
		Think:     ai.think,
		Options:   chatReqOptions{NumPredict: numPredict, Temperature: ai.temperature, Seed: ai.seed},
	})
	if err != nil {
		return "", chatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ai.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", chatResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", chatResponse{}, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", chatResponse{}, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", chatResponse{}, fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(cr.Message.Content), cr, nil
}

const thinkOpenTag, thinkCloseTag = "<think>", "</think>"

func splitThinking(reply string) (clean, thinking string) {
	start := strings.Index(reply, thinkOpenTag)
	if start < 0 {
		if end := strings.Index(reply, thinkCloseTag); end >= 0 {
			return strings.TrimSpace(reply[end+len(thinkCloseTag):]), strings.TrimSpace(reply[:end])
		}
		return reply, ""
	}
	contentStart := start + len(thinkOpenTag)
	end := strings.Index(reply[contentStart:], thinkCloseTag)
	if end < 0 {
		return "", strings.TrimSpace(reply[contentStart:])
	}
	end += contentStart
	clean = strings.TrimSpace(reply[:start] + reply[end+len(thinkCloseTag):])
	thinking = strings.TrimSpace(reply[contentStart:end])
	return clean, thinking
}

// withThinking prepends a model's raw chain-of-thought to the reasoning
// summary derived from its final answer, so both reach the reasoning log.
func withThinking(thinking, reasoning string) string {
	if thinking == "" {
		return reasoning
	}
	if reasoning != "" {
		reasoning = "\n\nDecision: " + reasoning
	}
	return "Thinking: " + thinking + reasoning
}

// GiveClue implements codenames.Spymaster.
func (ai *AI) GiveClue(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, error) {
	clue, _, err := ai.giveClue(b, agent)
	return clue, err
}

// GiveClueWithReasoning is like GiveClue, but also returns a human-readable
// explanation of why the clue was chosen.
func (ai *AI) GiveClueWithReasoning(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, string, error) {
	return ai.giveClue(b, agent)
}

func (ai *AI) giveClue(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, string, error) {
	teamName := "Red"
	if agent == codenames.BlueAgent {
		teamName = "Blue"
	}

	var myWords, opponentWords, bystanders, assassin []string
	opponent := codenames.RedAgent
	if agent == codenames.RedAgent {
		opponent = codenames.BlueAgent
	}

	for _, card := range b.Cards {
		if card.Revealed {
			continue
		}
		switch card.Agent {
		case agent:
			myWords = append(myWords, card.Codename)
		case opponent:
			opponentWords = append(opponentWords, card.Codename)
		case codenames.Bystander:
			bystanders = append(bystanders, card.Codename)
		case codenames.Assassin:
			assassin = append(assassin, card.Codename)
		}
	}

	system := fmt.Sprintf(`You are a skilled human playing as a Codenames spymaster. You give a single-word clue and name exactly which of your team's words that clue points to. Give clues the way a person would, not like a search engine.

Rules:
- Your clue must be a SINGLE word (no spaces, no hyphens, no proper nouns).
- Your clue can NEVER be a word that appears anywhere on the board, in any form, including as a substring. If "king" is on the board, then "king", "kings" and "kingdom" are all rejected — say "monarch" instead. If "ship" is on the board, then "shipping" and "worship" are rejected too, even though "ship" isn't a prefix in "worship" — say "vessel" or "devoted" instead. This applies to every word listed below, including your own team's words: naming a board word as the clue is an illegal move, not a shortcut.
- You MUST avoid clues that relate to the assassin word — guessing it loses the game instantly.
- You should avoid clues that relate to opponent words or bystanders.
- Every word you list as a target must be one of YOUR team's words, spelled exactly as given.
- 2 targets is a completely good, sufficient clue — it is not a lesser result. Only add a 3rd word if its link to the clue is just as strong as the other two. Never pad a 2-word clue with a weaker 3rd word merely to raise the count; a strong 2-word clue beats a padded 3-word one every time.
- Choose clues that feel intuitive and slightly creative, not just the most statistically obvious connection. Connecting words in an indirect or cultural way — the way a person would think of them — is good.
- Weigh the score. If your team has notably more words left than the opponent, play it safe — a smaller, high-confidence clue that guarantees progress beats a big swing you might blow. If you're notably behind, it's worth the extra risk: a clue targeting more words at once, even if less certain, gives you a chance to catch up that a safe 1-word clue doesn't.

%s

ASSASSIN: <clear, or the risk this clue runs toward the assassin word>
CLUE: <your one-word clue>
TARGETS: <comma-separated board words this clue covers>
NUMBER: <the count of words listed in TARGETS>

ASSASSIN: must never be empty — state plainly whether the clue is clear of the assassin word or explain the risk it runs.

Only list a word in TARGETS if you are highly confident an operative will reach it from your clue. Uncertainty means listing fewer words, never listing a word you are hoping about.

Example:
ASSASSIN: clear, no relation to "shadow"

CLUE: ocean
TARGETS: whale, ship
NUMBER: 2

`, ai.deliberationInstructions())

	prompt := fmt.Sprintf(`You are the %s team spymaster.

Score: your team has %d words left to find; the opponent has %d words left.

Your team's words (you want these guessed): %s
Opponent's words (avoid these): %s
Bystanders (avoid these): %s
Assassin (NEVER clue toward this): %s

Give your clue:`, teamName,
		len(myWords), len(opponentWords),
		strings.Join(myWords, ", "),
		strings.Join(opponentWords, ", "),
		strings.Join(bystanders, ", "),
		strings.Join(assassin, ", "))

	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	}

	// All retries share one timeout budget, matching the operative path: a
	// rejected reply must never multiply how long a human waits for a clue.
	ctx, cancel := context.WithTimeout(context.Background(), ai.timeout)
	defer cancel()

	var lastErr error
	// Grows on a truncated-thinking retry (see below) so a model that ran out
	// of budget mid-thought gets more room next time instead of hitting the
	// same ceiling again.
	numPredict := ai.maxTokens
	for attempt := range 3 {
		
		raw, cr, err := ai.chat(ctx, messages, "", numPredict)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && lastErr != nil {
				// We had a real reply earlier and merely ran out of time
				// re-asking; report the substantive rejection instead.
				break
			}
			return nil, "", fmt.Errorf("llm chat: %w", err)
		}

		
		log.Printf("[LLM Spymaster] attempt=%d, raw response: %q", attempt+1, raw)
		reply, thinking := splitThinking(raw)
		log.Printf("[LLM Spymaster] attempt=%d, eval_count=%d prompt_eval_count=%d total_duration=%s, thinking_chars=%d reply_chars=%d",
			attempt+1, cr.EvalCount, cr.PromptEvalCount, time.Duration(cr.TotalDuration), len(thinking), len(reply))

		if reply == "" && thinking != "" {
			
			numPredict = min(numPredict*2, 4*DefaultMaxTokens)
			lastErr = fmt.Errorf("model exhausted its %d-token budget before finishing its <think> block", numPredict/2)
			log.Printf("[LLM Spymaster] rejected attempt=%d: %v; retrying with numPredict=%d", attempt+1, lastErr, numPredict)
			messages = append(messages,
				chatMessage{Role: "user", Content: "Your previous reply ran out of budget before you reached an answer. Be more concise in your reasoning and make sure you output the final ASSASSIN/CLUE/TARGETS/NUMBER schema."},
			)
			continue
		}

		clue, reasoning, err := parseClueResponse(reply, myWords, b.Cards)
		if err == nil {
			log.Printf("[LLM Spymaster] clue: %s %d (targets: %s)", clue.Word, clue.Count, reasoning)
			return clue, withThinking(thinking, reasoning), nil
		}

		lastErr = err
		log.Printf("[LLM Spymaster] rejected attempt=%d: %v", attempt+1, err)

		// Tell the model exactly what was wrong so the retry is informed
		// rather than a re-roll of the same mistake.
		messages = append(messages,
			chatMessage{Role: "assistant", Content: reply},
			chatMessage{Role: "user", Content: fmt.Sprintf("That reply was rejected: %v. Respond again following the exact schema: ASSASSIN, then CLUE/TARGETS/NUMBER lines where NUMBER matches the count of words in TARGETS. Your team's words are: %s", err, strings.Join(myWords, ", "))},
		)
	}

	return nil, "", fmt.Errorf("no valid clue after 3 attempts: %w", lastErr)
}

// extractReason returns the text following a "REASON:" line in an LLM reply,
// or "" if no such line is present.
func extractReason(reply string) string {
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.IndexByte(line, ':'); idx > 0 && strings.EqualFold(line[:idx], "reason") {
			return strings.TrimSpace(line[idx+1:])
		}
	}
	return ""
}

// labeledLine returns the trimmed text following "<label>:" on the first
// line of reply that starts with that label (case-insensitive), or "" if no
// such line exists.
func labeledLine(reply, label string) string {
	prefix := strings.ToUpper(label) + ":"
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}


func parseClueResponse(reply string, myWords []string, board []codenames.Card) (*codenames.Clue, string, error) {
	assassinLine := strings.TrimSpace(labeledLine(reply, "ASSASSIN"))
	if assassinLine == "" {
		return nil, "", errors.New("reply had no ASSASSIN: line, or it was empty")
	}

	word := strings.ToLower(strings.TrimSpace(labeledLine(reply, "CLUE")))
	if word == "" {
		return nil, "", errors.New("CLUE: was empty")
	}
	if strings.ContainsAny(word, " \t-_") {
		return nil, "", fmt.Errorf("clue %q must be a single word", word)
	}
	if conflict, ok := codenames.ConflictingBoardWord(word, board); ok {
		return nil, "", fmt.Errorf("clue %q is or contains the board word %q; clues may never be words on the board", word, conflict)
	}

	targetsLine := labeledLine(reply, "TARGETS")
	if targetsLine == "" {
		return nil, "", errors.New("TARGETS: was empty; list the words your clue points to")
	}
	rawTargets := strings.Split(targetsLine, ",")

	numberLine := labeledLine(reply, "NUMBER")
	number, err := strconv.Atoi(strings.TrimSpace(numberLine))
	if err != nil {
		return nil, "", fmt.Errorf("NUMBER: %q was not an integer", numberLine)
	}

	// Canonicalize each target to the board's own spelling, rejecting anything
	// that isn't one of this team's unrevealed words.
	byLower := make(map[string]string, len(myWords))
	for _, w := range myWords {
		byLower[strings.ToLower(strings.TrimSpace(w))] = w
	}

	targets := make([]string, 0, len(rawTargets))
	seen := make(map[string]bool, len(rawTargets))
	for _, t := range rawTargets {
		key := strings.ToLower(strings.TrimSpace(t))
		if key == "" {
			continue
		}
		canonical, ok := byLower[key]
		if !ok {
			return nil, "", fmt.Errorf("target %q is not one of your team's words", t)
		}
		if seen[key] {
			return nil, "", fmt.Errorf("target %q was listed twice", t)
		}
		seen[key] = true
		targets = append(targets, canonical)
	}
	if len(targets) == 0 {
		return nil, "", errors.New("TARGETS: was empty; list the words your clue points to")
	}

	// NUMBER must agree with the words actually listed in TARGETS — this
	// catches a model that miscounts rather than silently trusting either
	// value on its own.
	if number != len(targets) {
		return nil, "", fmt.Errorf("NUMBER: %d does not match the %d word(s) listed in TARGETS", number, len(targets))
	}

	reasoning := fmt.Sprintf("Assassin check: %s\n\nTargets: %s",
		assassinLine, strings.Join(targets, ", "))

	return &codenames.Clue{Word: word, Count: len(targets)}, reasoning, nil
}


type GuessDecisionConfig struct {
	MandatedThreshold float64
	BonusThreshold float64
	RiskiestWordPenalty float64
	LinkTypeCaps map[string]float64
	UnknownLinkTypeCap float64
}

var DefaultGuessDecisionConfig = GuessDecisionConfig{
	MandatedThreshold:   0.55,
	BonusThreshold:      0.65,
	RiskiestWordPenalty: 0.15,
	LinkTypeCaps: map[string]float64{
		"direct":    1.00,
		"category":  0.75,
		"idiom":     0.35,
		"multi_hop": 0.40,
	},
	UnknownLinkTypeCap: 0.35,
}

// Candidate is one board word the operative considered, with its calibrated
// confidence that the clue is pointing at it.
type Candidate struct {
	Word       string  `json:"word"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
	LinkType   string  `json:"link_type"`
	RawConfidence float64 `json:"-"`
}

type CandidateResponse struct {
	Candidates             []Candidate `json:"candidates"`
	RiskiestBoardWord      string      `json:"riskiest_board_word"`
	TopCandidateIsRiskiest bool        `json:"top_candidate_is_riskiest"`
}

type GuessResult struct {
	
	Guess       string
	RawResponse string
	Candidates  []Candidate
	RiskiestBoardWord      string
	TopCandidateIsRiskiest bool
	ThresholdApplied float64
	MustGuess        bool
	GuessesThisTurn  int
	ClueNumber       int
	ParseError bool
	CapApplied bool
}

// Guess implements codenames.Operative.
func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	res, err := ai.GuessWithCandidates(b, c, true /* mustGuess */, 0, nil)
	if err != nil {
		return "", err
	}
	return res.Guess, nil
}

// GuessOrPass is like Guess, but when mustGuess is false it may return
// codenames.PassGuess to end the turn instead of risking a bad guess.
func (ai *AI) GuessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, error) {
	res, err := ai.GuessWithCandidates(b, c, mustGuess, 0, nil)
	if err != nil {
		return "", err
	}
	return res.Guess, nil
}

// GuessOrPassWithReasoning is like GuessOrPass, but also returns a
// human-readable rendering of the candidates considered.
func (ai *AI) GuessOrPassWithReasoning(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, string, error) {
	res, err := ai.GuessWithCandidates(b, c, mustGuess, 0, nil)
	if err != nil {
		return "", "", err
	}
	return res.Guess, renderCandidateReasoning(res), nil
}

// renderCandidateReasoning turns a GuessResult's structured candidates into
// the human-readable reasoning string the admin UI / reasoning log expects,
// mirroring the "Assassin check: ...\n\nTargets: ..." style used for clues.
func renderCandidateReasoning(res *GuessResult) string {
	if res.ParseError {
		return "no reasoning available: every attempt failed to produce parseable candidates"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Threshold: %.2f (mustGuess=%v)\n", res.ThresholdApplied, res.MustGuess)
	fmt.Fprintf(&b, "Riskiest board word: %s (top candidate is riskiest: %v)\n\nCandidates:\n", res.RiskiestBoardWord, res.TopCandidateIsRiskiest)
	for _, c := range res.Candidates {
		if c.Confidence < c.RawConfidence {
			fmt.Fprintf(&b, "- %s (confidence %.2f, capped from %.2f, %s): %s\n", c.Word, c.Confidence, c.RawConfidence, c.LinkType, c.Reasoning)
		} else {
			fmt.Fprintf(&b, "- %s (confidence %.2f, %s): %s\n", c.Word, c.Confidence, c.LinkType, c.Reasoning)
		}
	}
	return strings.TrimSpace(b.String())
}

// GuessWithCandidates asks the operative for ranked, confidence-scored candidates 
func (ai *AI) GuessWithCandidates(b *codenames.Board, c *codenames.Clue, mustGuess bool, guessesThisTurn int, revealedHistory []string) (*GuessResult, error) {
	var unrevealed []string
	for _, card := range b.Cards {
		if !card.Revealed {
			unrevealed = append(unrevealed, card.Codename)
		}
	}

	system := fmt.Sprintf(`You are the operative (guesser) in a game of Codenames.

Your job is NOT to pick a word. Your job is to report what you actually believe, with honest confidence. A separate system decides whether to guess or pass.

Return the 3 strongest candidates, ranked. For each, classify how the clue connects to it, and give your own honest confidence (0.0 = no real basis, 1.0 = certain) for how sure you are the clue means that word:

  direct     A direct synonym, category member, or definitional link.
  category   A real but broader kind-of/type-of relationship — not a synonym, but a genuine category link.
  idiom      The link only exists through a specific fixed phrase, pun, or figure of speech.
  multi_hop  The link only holds after two or more separate associative steps.

Calibration rules:
- Do not inflate. If a turn has no strong candidate, the correct output is three low-confidence candidates. That is a valid and useful answer.
- An idiom or multi_hop link is inherently less reliable than a direct or category link. Your confidence should reflect that difference honestly — don't treat a clever idiom as a near-certainty just because it's the best thing you found.
- Confidence should vary from candidate to candidate based on how sure you actually are about each one specifically. Don't give multiple candidates the same confidence just because they share a link type.
- If several candidates are genuinely comparable, giving them similar, middling confidence is a legitimate answer — that's honest reporting, not indecision.

Then, separately: look at every unrevealed word on the board and name the single one you would most expect the spymaster to be avoiding, given how dangerous a wrong hit would be. State whether any of your three candidates is that word.

%s

Respond with a single JSON object and nothing else — no prose before it, no commentary after it, no markdown fences:

{
  "candidates": [
    {"word": "...", "confidence": 0.0, "reasoning": "one sentence", "link_type": "direct|category|idiom|multi_hop"}
  ],
  "riskiest_board_word": "...",
  "top_candidate_is_riskiest": true|false
}`, ai.guessFormatInstructions())

	revealedStr := "(none yet)"
	if len(revealedHistory) > 0 {
		revealedStr = strings.Join(revealedHistory, ", ")
	}
	prompt := fmt.Sprintf(`BOARD (unrevealed words):
%s

REVEALED SO FAR:
%s

CLUE: %s %d
GUESSES ALREADY MADE THIS TURN: %d`, strings.Join(unrevealed, ", "), revealedStr, c.Word, c.Count, guessesThisTurn)

	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	}

	// All retries share one timeout budget, so a slow model can't multiply
	// the wait by re-trying — after this, we fall back to a random legal
	// guess. Capped tighter than the clue budget, but note this (like
	// DefaultTimeout above) no longer fits under humanThinkDelay's ~15s
	// guess window in cmd/ai-server/server.go once a reasoning model uses
	// its full budget.
	guessBudget := min(ai.timeout, 90*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), guessBudget)
	defer cancel()

	// Starts well below ai.maxTokens (see DefaultGuessMaxTokens) so an
	// unproductive, narrating attempt fails and retries faster instead of
	// burning a full clue-sized budget on prose. Grows on a truncated-thinking
	// retry (see below), same as giveClue, so a model that runs out of budget
	// mid-<think> gets more room instead of silently falling back to a random
	// guess.
	numPredict := DefaultGuessMaxTokens

	base := &GuessResult{MustGuess: mustGuess, GuessesThisTurn: guessesThisTurn, ClueNumber: c.Count}

	// Try up to 3 times to get parseable candidates.
	for attempt := range 3 {
		raw, cr, err := ai.chat(ctx, messages, "", numPredict)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[LLM Operative] clue=%q timed out after attempt=%d, falling back", c.Word, attempt+1)
				base.ParseError = true
				return base, nil
			}
			return nil, fmt.Errorf("llm chat: %w", err)
		}

		// See the matching comment in giveClue: logged pre-split so this
		// stays a true raw response for merge_reasoning_data.py.
		log.Printf("[LLM Operative] clue=%q, attempt=%d, raw response: %q", c.Word, attempt+1, raw)
		reply, thinking := splitThinking(raw)
		log.Printf("[LLM Operative] clue=%q, attempt=%d, eval_count=%d prompt_eval_count=%d total_duration=%s, thinking_chars=%d reply_chars=%d",
			c.Word, attempt+1, cr.EvalCount, cr.PromptEvalCount, time.Duration(cr.TotalDuration), len(thinking), len(reply))

		if reply == "" && thinking != "" {
			
			numPredict = min(numPredict*2, 4*DefaultMaxTokens)
			log.Printf("[LLM Operative] clue=%q attempt=%d exhausted its token budget before finishing its <think> block; retrying with numPredict=%d", c.Word, attempt+1, numPredict)
			messages = append(messages,
				chatMessage{Role: "user", Content: "Your previous reply ran out of budget before you reached an answer. Be more concise and make sure you output the JSON candidates object."},
			)
			continue
		}

		resp, parseErr := parseCandidateResponse(reply, unrevealed)
		if parseErr != nil {
			log.Printf("[LLM Operative] clue=%q attempt=%d rejected: %v", c.Word, attempt+1, parseErr)
			messages = append(messages,
				chatMessage{Role: "assistant", Content: reply},
				chatMessage{Role: "user", Content: fmt.Sprintf("That reply was rejected: %v. Respond again with ONLY the JSON candidates object, using words exactly as they appear on the board: %s", parseErr, strings.Join(unrevealed, ", "))},
			)
			continue
		}

	
		capApplied, unknownLinkTypes := applyLinkTypeCaps(ai.guessConfig, resp.Candidates)
		if unknownLinkTypes > 0 {
			ai.unknownLinkTypeCount.Add(int64(unknownLinkTypes))
		}

		top := topCandidate(resp.Candidates)
		base.RawResponse = raw
		base.Candidates = resp.Candidates
		base.RiskiestBoardWord = resp.RiskiestBoardWord
		base.TopCandidateIsRiskiest = resp.TopCandidateIsRiskiest
		base.CapApplied = capApplied
		base.Guess, base.ThresholdApplied = decideGuess(ai.guessConfig, mustGuess, guessesThisTurn, c.Count, top, resp.TopCandidateIsRiskiest)

		log.Printf("[LLM Operative] clue=%q decision=%q top=%q raw_confidence=%.2f confidence=%.2f link_type=%q cap_applied=%v threshold=%.2f",
			c.Word, base.Guess, top.Word, top.RawConfidence, top.Confidence, top.LinkType, capApplied, base.ThresholdApplied)
		return base, nil
	}


	ai.parseErrorCount.Add(1)
	log.Printf("[LLM Operative] all retries failed to parse candidates for clue %q, falling back (parse_error_count=%d)", c.Word, ai.parseErrorCount.Load())
	base.ParseError = true
	return base, nil
}

func (ai *AI) ParseErrorCount() int64 {
	return ai.parseErrorCount.Load()
}

func (ai *AI) UnknownLinkTypeCount() int64 {
	return ai.unknownLinkTypeCount.Load()
}

func applyLinkTypeCaps(cfg GuessDecisionConfig, candidates []Candidate) (capApplied bool, unknownLinkTypes int) {
	for i := range candidates {
		c := &candidates[i]
		limit, ok := cfg.LinkTypeCaps[strings.ToLower(strings.TrimSpace(c.LinkType))]
		if !ok {
			limit = cfg.UnknownLinkTypeCap
			unknownLinkTypes++
		}
		if c.Confidence > limit {
			c.Confidence = limit
			capApplied = true
		}
	}
	return capApplied, unknownLinkTypes
}

func topCandidate(candidates []Candidate) Candidate {
	top := candidates[0]
	for _, c := range candidates[1:] {
		if c.Confidence > top.Confidence {
			top = c
		}
	}
	return top
}

func decideGuess(cfg GuessDecisionConfig, mustGuess bool, guessesThisTurn, clueNumber int, top Candidate, topIsRiskiest bool) (string, float64) {
	if mustGuess {
		return top.Word, 0
	}

	threshold := cfg.MandatedThreshold
	if guessesThisTurn >= clueNumber {
		threshold = cfg.BonusThreshold
	}
	if topIsRiskiest {
		threshold += cfg.RiskiestWordPenalty
	}

	// <=, not <: a candidate sitting exactly on the bar passes rather than
	// guesses — ties go to caution, not confidence.
	if top.Confidence <= threshold {
		return codenames.PassGuess, threshold
	}
	return top.Word, threshold
}

// stripCodeFences removes a wrapping ```json ... ``` or ``` ... ``` block,
// which local models emit around JSON regardless of being told not to.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if nl := strings.IndexByte(s, '\n'); nl >= 0 && strings.TrimSpace(s[:nl]) != "" {
		// Leading language tag on the fence line (e.g. "json") — drop it.
		s = s[nl+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

func extractJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}


func parseCandidateResponse(reply string, unrevealed []string) (*CandidateResponse, error) {
	cleaned := stripCodeFences(reply)

	var resp CandidateResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		// Fall back to scanning for a balanced {...} in case of surrounding
		// prose the model wasn't supposed to add.
		obj, ok := extractJSONObject(cleaned)
		if !ok {
			return nil, fmt.Errorf("no JSON object found in reply: %w", err)
		}
		if err := json.Unmarshal([]byte(obj), &resp); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
	}

	if len(resp.Candidates) == 0 {
		return nil, errors.New("candidates was empty")
	}

	byLower := make(map[string]string, len(unrevealed))
	for _, w := range unrevealed {
		byLower[strings.ToLower(strings.TrimSpace(w))] = w
	}

	valid := make([]Candidate, 0, len(resp.Candidates))
	for _, c := range resp.Candidates {
		canonical, ok := byLower[strings.ToLower(strings.TrimSpace(c.Word))]
		if !ok {
			// A hallucinated word is dropped, not a hard reject on its own —
			// the model may still have reported other, valid candidates. Only
			// an empty result after filtering is a parse error (below).
			log.Printf("[LLM Operative] dropping hallucinated candidate %q (not an unrevealed board word)", c.Word)
			continue
		}
		c.Word = canonical
		c.Confidence = clampConfidence(c.Confidence)
		c.RawConfidence = c.Confidence
		valid = append(valid, c)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("no candidate word matched an unrevealed board word (got %d hallucinated)", len(resp.Candidates))
	}
	resp.Candidates = valid

	if canonical, ok := byLower[strings.ToLower(strings.TrimSpace(resp.RiskiestBoardWord))]; ok {
		resp.RiskiestBoardWord = canonical
	}

	return &resp, nil
}

func clampConfidence(c float64) float64 {
	return min(1, max(0, c))
}

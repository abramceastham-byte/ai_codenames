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
	verboseLogs          bool
	endpoint             string // e.g. "http://localhost:11434"
	model                string // e.g. "llama3"
	timeout              time.Duration
	maxTokens            int
	temperature          float64
	seed                 *int
	think                *bool
	guessConfig          GuessDecisionConfig
	parseErrorCount      atomic.Int64
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

// WithVerboseLogs re-enables the full per-attempt dumps — the model's entire
// raw reply and its token/duration counters. They're off by default because a
// reasoning model's reply is thousands of characters of <think>, which buries
// the summary blocks that make a running game readable. Rejected attempts are
// always logged regardless of this setting, since a rejection is exactly when
// the raw text is worth seeing.
func WithVerboseLogs(verbose bool) Option {
	return func(ai *AI) { ai.verboseLogs = verbose }
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
	Model     string         `json:"model"`
	Messages  []chatMessage  `json:"messages"`
	Stream    bool           `json:"stream"`
	Format    string         `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Think     *bool          `json:"think,omitempty"`
	Options   chatReqOptions `json:"options"`
}

type chatReqOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature"`
	Seed        *int    `json:"seed,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Thinking carries a reasoning model's deliberation when Ollama is asked
	// for it via the top-level "think" field. Models differ in where they put
	// it: qwq and friends inline it into Content wrapped in <think> tags,
	// while a model driven by "think": true returns it here with Content
	// holding only the final answer. Both paths have to be read — see
	// thinkingFrom — or a reply that spent its whole budget deliberating
	// looks indistinguishable from an empty one.
	Thinking string `json:"thinking"`
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

// thinkingFrom returns the model's deliberation and its final answer,
// accepting either convention: inline <think> tags inside the content, or
// Ollama's separate "thinking" field. Without the second case, a model that
// exhausts num_predict before emitting any answer reports empty content AND
// empty thinking, so the truncated-thinking retry below never fires and all
// three attempts fail identically on the same budget.
func thinkingFrom(raw string, msg chatMessage) (reply, thinking string) {
	reply, thinking = splitThinking(raw)
	if thinking == "" {
		thinking = msg.Thinking
	}
	return reply, thinking
}

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
WHY: <one per target, as "word - short reason", separated by semicolons>
NUMBER: <the count of words listed in TARGETS>

ASSASSIN: must never be empty — state plainly whether the clue is clear of the assassin word or explain the risk it runs.

WHY: one short phrase per target saying how the clue reaches that word — a handful of words each, not a sentence. "whale - sea mammal" is right; a full explanation of your deliberation is not.

Only list a word in TARGETS if you are highly confident an operative will reach it from your clue. Uncertainty means listing fewer words, never listing a word you are hoping about.

Real players don't all play the same way — some examples of the range that's normal:

Example (steady, dictionary-ish link):
ASSASSIN: clear, no relation to "shadow"

CLUE: ocean
TARGETS: whale, ship
WHY: whale - sea mammal; ship - sails the ocean
NUMBER: 2

Example (greedy, taking a 3rd word on a slightly looser link because the team is behind):
ASSASSIN: clear, no relation to "spider"

CLUE: royal
TARGETS: crown, palace, jack
WHY: crown - worn by royalty; palace - where royals live; jack - royal face card
NUMBER: 3

Example (pop-culture/idiomatic link instead of a dictionary one):
ASSASSIN: clear, no relation to "night"

CLUE: krypton
TARGETS: superman, cape
WHY: superman - from Krypton; cape - what he wears
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

		reply, thinking := thinkingFrom(raw, cr.Message)
		if ai.verboseLogs {
			log.Printf("[LLM Spymaster] attempt=%d, raw response: %q", attempt+1, raw)
			log.Printf("[LLM Spymaster] attempt=%d, eval_count=%d prompt_eval_count=%d total_duration=%s, thinking_chars=%d reply_chars=%d",
				attempt+1, cr.EvalCount, cr.PromptEvalCount, time.Duration(cr.TotalDuration), len(thinking), len(reply))
		}

		if reply == "" && thinking != "" {
			// The ceiling scales with this AI's configured budget, not the
			// package default: raising maxTokens to work around a model that
			// deliberates at length was previously almost useless, because
			// growth still stopped at 4*DefaultMaxTokens. Report the budget
			// that actually ran out rather than deriving it from the new
			// value, which is wrong whenever the ceiling clamps.
			exhausted := numPredict
			numPredict = min(numPredict*2, 4*ai.maxTokens)
			lastErr = fmt.Errorf("model exhausted its %d-token budget before finishing its <think> block", exhausted)
			log.Printf("[LLM Spymaster] rejected attempt=%d: %v; retrying with numPredict=%d", attempt+1, lastErr, numPredict)
			messages = append(messages,
				chatMessage{Role: "user", Content: "Your previous reply ran out of budget before you reached an answer. Be more concise in your reasoning and make sure you output the final ASSASSIN/CLUE/TARGETS/WHY/NUMBER schema."},
			)
			continue
		}

		p, err := parseClue(reply, myWords, b.Cards)
		if err == nil {
			log.Print(spymasterSummary(teamName, p))
			return p.Clue, withThinking(thinking, p.Reasoning), nil
		}

		lastErr = err
		log.Printf("[LLM Spymaster] rejected attempt=%d: %v", attempt+1, err)

		// Tell the model exactly what was wrong so the retry is informed
		// rather than a re-roll of the same mistake.
		messages = append(messages,
			chatMessage{Role: "assistant", Content: reply},
			chatMessage{Role: "user", Content: fmt.Sprintf("That reply was rejected: %v. Respond again following the exact schema: ASSASSIN, then CLUE/TARGETS/WHY/NUMBER lines where NUMBER matches the count of words in TARGETS. Your team's words are: %s", err, strings.Join(myWords, ", "))},
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

// targetWhy pairs one target word with the spymaster's short rationale for
// it. Why is empty when the model omitted or malformed its WHY line — that's
// tolerated rather than rejected (see parseClueResponse), so consumers must
// handle an empty Why.
type targetWhy struct {
	Word string
	Why  string
}

// clueParse is everything parseClueResponse recovers from a valid reply: the
// move itself, the per-target rationale behind it, and the human-readable
// reasoning string logged to logs/ai_reasoning.jsonl and shown in the admin UI.
type clueParse struct {
	Clue      *codenames.Clue
	Targets   []targetWhy
	Assassin  string
	Reasoning string
}

func parseClueResponse(reply string, myWords []string, board []codenames.Card) (*codenames.Clue, string, error) {
	p, err := parseClue(reply, myWords, board)
	if err != nil {
		return nil, "", err
	}
	return p.Clue, p.Reasoning, nil
}

func parseClue(reply string, myWords []string, board []codenames.Card) (*clueParse, error) {
	assassinLine := strings.TrimSpace(labeledLine(reply, "ASSASSIN"))
	if assassinLine == "" {
		return nil, errors.New("reply had no ASSASSIN: line, or it was empty")
	}

	word := strings.ToLower(strings.TrimSpace(labeledLine(reply, "CLUE")))
	if word == "" {
		return nil, errors.New("CLUE: was empty")
	}
	if strings.ContainsAny(word, " \t-_") {
		return nil, fmt.Errorf("clue %q must be a single word", word)
	}
	if conflict, ok := codenames.ConflictingBoardWord(word, board); ok {
		return nil, fmt.Errorf("clue %q is or contains the board word %q; clues may never be words on the board", word, conflict)
	}

	targetsLine := labeledLine(reply, "TARGETS")
	if targetsLine == "" {
		return nil, errors.New("TARGETS: was empty; list the words your clue points to")
	}
	rawTargets := strings.Split(targetsLine, ",")

	numberLine := labeledLine(reply, "NUMBER")
	number, err := strconv.Atoi(strings.TrimSpace(numberLine))
	if err != nil {
		return nil, fmt.Errorf("NUMBER: %q was not an integer", numberLine)
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
			return nil, fmt.Errorf("target %q is not one of your team's words", t)
		}
		if seen[key] {
			return nil, fmt.Errorf("target %q was listed twice", t)
		}
		seen[key] = true
		targets = append(targets, canonical)
	}
	if len(targets) == 0 {
		return nil, errors.New("TARGETS: was empty; list the words your clue points to")
	}

	// NUMBER must agree with the words actually listed in TARGETS — this
	// catches a model that miscounts rather than silently trusting either
	// value on its own.
	if number != len(targets) {
		return nil, fmt.Errorf("NUMBER: %d does not match the %d word(s) listed in TARGETS", number, len(targets))
	}

	// WHY is deliberately parsed last and never rejected: it exists purely to
	// explain the move in the terminal summary and admin UI, so a model that
	// omits it or mangles its format should still get its clue accepted
	// rather than burn a retry on cosmetics.
	whys := parseWhyLine(labeledLine(reply, "WHY"), targets)

	var b strings.Builder
	fmt.Fprintf(&b, "Assassin check: %s\n\nTargets: %s", assassinLine, strings.Join(targets, ", "))
	for _, t := range whys {
		if t.Why != "" {
			fmt.Fprintf(&b, "\n  - %s: %s", t.Word, t.Why)
		}
	}

	return &clueParse{
		Clue:      &codenames.Clue{Word: word, Count: len(targets)},
		Targets:   whys,
		Assassin:  assassinLine,
		Reasoning: b.String(),
	}, nil
}

// parseWhyLine matches the spymaster's "word - reason; word - reason" WHY line
// back onto the canonical target list. Anything it can't match is dropped
// rather than surfaced: an unmatched or hallucinated word here would be a
// rationale for a word the clue doesn't actually target, which is worse than
// showing no rationale at all. Every target is always returned, in TARGETS
// order, so a partially-parsed WHY still yields a complete target list.
func parseWhyLine(line string, targets []string) []targetWhy {
	byLower := make(map[string]string, len(targets))
	for _, t := range targets {
		byLower[strings.ToLower(t)] = ""
	}

	for _, part := range strings.Split(line, ";") {
		word, why, ok := splitWhyPart(part)
		if !ok {
			continue
		}
		key := strings.ToLower(word)
		if _, isTarget := byLower[key]; !isTarget {
			continue
		}
		byLower[key] = why
	}

	out := make([]targetWhy, 0, len(targets))
	for _, t := range targets {
		out = append(out, targetWhy{Word: t, Why: byLower[strings.ToLower(t)]})
	}
	return out
}

// splitWhyPart splits one "word - reason" fragment. It accepts a hyphen, en
// dash, em dash or colon as the separator, since models reach for all four
// regardless of which one the prompt shows.
func splitWhyPart(part string) (word, why string, ok bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return "", "", false
	}
	for _, sep := range []string{" - ", " – ", " — ", ":", "-", "–", "—"} {
		if word, why, found := strings.Cut(part, sep); found {
			word, why = strings.TrimSpace(word), strings.TrimSpace(why)
			if word != "" && why != "" {
				return word, why, true
			}
		}
	}
	return "", "", false
}

type GuessDecisionConfig struct {
	MandatedThreshold   float64
	BonusThreshold      float64
	RiskiestWordPenalty float64
	LinkTypeCaps        map[string]float64
	UnknownLinkTypeCap  float64
}

var DefaultGuessDecisionConfig = GuessDecisionConfig{
	MandatedThreshold:   0.55,
	BonusThreshold:      0.80,
	RiskiestWordPenalty: 0.15,
	// Caps discount link types the model tends to be overconfident about,
	// but they must stay ABOVE MandatedThreshold or they stop being a
	// discount and become a categorical ban: a capped value can never exceed
	// the threshold, so the candidate can never be guessed after the first
	// (unthresholded) guess no matter how certain the model is. idiom (0.35)
	// and multi_hop (0.40) both sat below 0.55 and were unreachable that way
	// — an operative that correctly read "lock on" as the idiom behind the
	// clue AIM had that candidate demoted below a wrong category match. The
	// ordering direct > category > idiom > multi_hop is preserved; only the
	// floor moved above the threshold.
	LinkTypeCaps: map[string]float64{
		"direct":    1.00,
		"category":  0.75,
		"idiom":     0.65,
		"multi_hop": 0.60,
	},
	UnknownLinkTypeCap: 0.35,
}

// Candidate is one board word the operative considered, with its calibrated
// confidence that the clue is pointing at it.
type Candidate struct {
	Word          string  `json:"word"`
	Confidence    float64 `json:"confidence"`
	Reasoning     string  `json:"reasoning"`
	LinkType      string  `json:"link_type"`
	RawConfidence float64 `json:"-"`
}

type CandidateResponse struct {
	Candidates             []Candidate `json:"candidates"`
	RiskiestBoardWord      string      `json:"riskiest_board_word"`
	TopCandidateIsRiskiest bool        `json:"top_candidate_is_riskiest"`
}

type GuessResult struct {
	Guess                  string
	RawResponse            string
	Candidates             []Candidate
	RiskiestBoardWord      string
	TopCandidateIsRiskiest bool
	ThresholdApplied       float64
	MustGuess              bool
	GuessesThisTurn        int
	ClueNumber             int
	ParseError             bool
	CapApplied             bool
}

// Guess implements codenames.Operative.
func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	res, err := ai.GuessWithCandidates(b, c, "" /* team */, true /* mustGuess */, 0, nil)
	if err != nil {
		return "", err
	}
	return res.Guess, nil
}

// GuessOrPass is like Guess, but when mustGuess is false it may return
// codenames.PassGuess to end the turn instead of risking a bad guess.
func (ai *AI) GuessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, error) {
	res, err := ai.GuessWithCandidates(b, c, "" /* team */, mustGuess, 0, nil)
	if err != nil {
		return "", err
	}
	return res.Guess, nil
}

// GuessOrPassWithReasoning is like GuessOrPass, but also returns a
// human-readable rendering of the candidates considered.
func (ai *AI) GuessOrPassWithReasoning(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, string, error) {
	res, err := ai.GuessWithCandidates(b, c, "" /* team */, mustGuess, 0, nil)
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
// team labels the terminal summary block so two teams' operatives are
// distinguishable in one log stream; it is purely cosmetic and may be empty
// (the plain Operative entry points below pass "", having no team context).
func (ai *AI) GuessWithCandidates(b *codenames.Board, c *codenames.Clue, team string, mustGuess bool, guessesThisTurn int, revealedHistory []string) (*GuessResult, error) {
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

The clue is the CLUE WORD line and nothing else. NUMBER OF TARGET WORDS is how many of your team's words the spymaster says that one word points to — it is a count, never part of the clue's meaning. A clue word of "power" with a count of 2 means "two of your words relate to POWER"; it does not mean "power of two".

Calibration rules:
- Do not inflate. If a turn has no strong candidate, the correct output is three low-confidence candidates. That is a valid and useful answer.
- An idiom or multi_hop link is inherently less reliable than a direct or category link. Your confidence should reflect that difference honestly — don't treat a clever idiom as a near-certainty just because it's the best thing you found.
- Confidence should vary from candidate to candidate based on how sure you actually are about each one specifically. Don't give multiple candidates the same confidence just because they share a link type.
- If several candidates are genuinely comparable, giving them similar, middling confidence is a legitimate answer — that's honest reporting, not indecision.

Then, separately: look at every unrevealed word on the board and name the single one you would most expect the spymaster to be avoiding, given how dangerous a wrong hit would be. State whether any of your three candidates is that word.

Real players don't only reach for the dictionary meaning — a clue like "krypton" might bring "superman" to mind through the movies long before "element" does. That's a legitimate idiom/multi_hop candidate, not a stretch, as long as your confidence for it honestly reflects that it's a looser link than a direct synonym would be.

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

CLUE WORD: %s
NUMBER OF TARGET WORDS: %d
GUESSES ALREADY MADE THIS TURN: %d`, strings.Join(unrevealed, ", "), revealedStr, c.Word, c.Count, guessesThisTurn)

	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	}

	// Half the clue budget, so a guess stays tighter than a clue while still
	// scaling with configuration. This was a hardcoded 90s, which silently
	// ignored a raised --ollama_timeout: under any load that pushed a call
	// past 90 seconds every guess timed out and fell back, and because the
	// fallback returns an empty guess, the turn simply ended. Degraded runs
	// looked like cautious passing rather than failure. DefaultTimeout is
	// 3 minutes, so the default behaviour here is unchanged at 90s.
	guessBudget := ai.timeout / 2
	ctx, cancel := context.WithTimeout(context.Background(), guessBudget)
	defer cancel()

	numPredict := DefaultGuessMaxTokens

	base := &GuessResult{MustGuess: mustGuess, GuessesThisTurn: guessesThisTurn, ClueNumber: c.Count}

	// Try up to 3 times to get parseable candidates.
	for attempt := range 3 {
		// "json" puts Ollama in structured-output mode, constraining
		// generation so the reply is always a syntactically valid JSON
		// object. The prompt already demands "a single JSON object and
		// nothing else", but asking is not enough: without this, a model
		// routinely opens with prose ("Okay, let's tackle this...") and the
		// whole attempt is thrown away on "no JSON object found". It does not
		// constrain the object's *shape*, so the field validation below still
		// does its job.
		raw, cr, err := ai.chat(ctx, messages, "json", numPredict)
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
		reply, thinking := thinkingFrom(raw, cr.Message)
		if ai.verboseLogs {
			log.Printf("[LLM Operative] clue=%q, attempt=%d, raw response: %q", c.Word, attempt+1, raw)
			log.Printf("[LLM Operative] clue=%q, attempt=%d, eval_count=%d prompt_eval_count=%d total_duration=%s, thinking_chars=%d reply_chars=%d",
				c.Word, attempt+1, cr.EvalCount, cr.PromptEvalCount, time.Duration(cr.TotalDuration), len(thinking), len(reply))
		}

		if reply == "" && thinking != "" {
			// Ceiling scales with the configured budget — see the matching
			// comment in giveClue.
			exhausted := numPredict
			numPredict = min(numPredict*2, 4*ai.maxTokens)
			log.Printf("[LLM Operative] clue=%q attempt=%d exhausted its %d-token budget before finishing its <think> block; retrying with numPredict=%d", c.Word, attempt+1, exhausted, numPredict)
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

		log.Print(operativeSummary(team, c, base))
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

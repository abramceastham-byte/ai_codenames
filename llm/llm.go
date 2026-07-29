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
	"strings"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

// DefaultTimeout bounds a single Ollama call (and the total budget across
// retries) so a slow model can't make an AI player wait far longer than the
// human-mimicking delay window applied by the caller.
const DefaultTimeout = 20 * time.Second

// AI implements codenames.Spymaster and codenames.Operative using a local
// Ollama model.
type AI struct {
	endpoint string // e.g. "http://localhost:11434"
	model    string // e.g. "llama3"
	timeout  time.Duration
}

func New(endpoint, model string, timeout time.Duration) *AI {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &AI{endpoint: endpoint, model: model, timeout: timeout}
}

// Ollama chat API types

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	// Format, when set to "json", makes Ollama constrain decoding to valid
	// JSON. It doesn't enforce our schema, but it removes the whole class of
	// "model wrapped the object in prose" parse failures.
	Format string `json:"format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// chat sends a conversation to Ollama. format is passed through to the API;
// pass "json" to constrain the reply to a JSON value, or "" for free text.
func (ai *AI) chat(ctx context.Context, messages []chatMessage, format string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    ai.model,
		Messages: messages,
		Stream:   false,
		Format:   format,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ai.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(cr.Message.Content), nil
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

	system := `You are a skilled human playing as a Codenames spymaster. You give a single-word clue and name exactly which of your team's words that clue points to. Give clues the way a person would, not like a search engine.

Rules:
- Your clue must be a SINGLE word (no spaces, no hyphens, no proper nouns).
- Your clue can NEVER be a word that appears anywhere on the board, in any form. If "king" is on the board, then "king", "kings" and "kingdom" are all rejected — say "monarch" instead. This applies to every word listed below, including your own team's words: naming a board word as the clue is an illegal move, not a shortcut.
- You MUST avoid clues that relate to the assassin word — guessing it loses the game instantly.
- You should avoid clues that relate to opponent words or bystanders.
- Every word you list in "targets" must be one of YOUR team's words, spelled exactly as given.
- Prefer 2 or 3 targets. Only list 4 or more if the connection is so natural that a person would spot it instantly.
- Choose clues that feel intuitive and slightly creative, not just the most statistically obvious connection. Connecting words in an indirect or cultural way — the way a person would think of them — is good.

Before finalizing your clue, explicitly ask yourself:
- Is this clue associated with the assassin word in meaning, sound, or category? If an operative might connect it to the assassin, discard it and choose a different clue.
- Could any of my target words be mistaken for the opponent's words?
- Is there any bystander or assassin word that shares my clue?
- If yes, drop the risky word from targets or choose a safer clue.

Only list a word in "targets" if you are highly confident an operative will reach it from your clue. Uncertainty means listing fewer words, never listing a word you are hoping about.

Respond with ONLY a JSON object, no other text, in exactly this shape:
{"clue": "<your one-word clue>", "targets": ["<board word>", ...], "links": {"<board word>": "<why the clue points to this specific word>", ...}}

"links" must contain one entry for every word in "targets" — the same words, no more and no fewer — and each value must be a concrete reason that word specifically is reached from the clue. A reply whose "links" keys do not exactly match "targets" is rejected.

Example:
{"clue": "ocean", "targets": ["whale", "ship"], "links": {"whale": "whales are the largest ocean animals", "ship": "ships cross the ocean"}}`

	prompt := fmt.Sprintf(`You are the %s team spymaster.

Your team's words (you want these guessed): %s
Opponent's words (avoid these): %s
Bystanders (avoid these): %s
Assassin (NEVER clue toward this): %s

Give your clue:`, teamName,
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
	for attempt := range 3 {
		reply, err := ai.chat(ctx, messages, "json")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && lastErr != nil {
				// We had a real reply earlier and merely ran out of time
				// re-asking; report the substantive rejection instead.
				break
			}
			return nil, "", fmt.Errorf("llm chat: %w", err)
		}

		log.Printf("[LLM Spymaster] attempt=%d, raw response: %q", attempt+1, reply)

		clue, reasoning, err := parseClueResponse(reply, myWords, b.Cards)
		if err == nil {
			log.Printf("[LLM Spymaster] clue: %s %d (targets: %s)", clue.Word, clue.Count, reasoning)
			return clue, reasoning, nil
		}

		lastErr = err
		log.Printf("[LLM Spymaster] rejected attempt=%d: %v", attempt+1, err)

		// Tell the model exactly what was wrong so the retry is informed
		// rather than a re-roll of the same mistake.
		messages = append(messages,
			chatMessage{Role: "assistant", Content: reply},
			chatMessage{Role: "user", Content: fmt.Sprintf("That reply was rejected: %v. Respond again with ONLY the JSON object, listing one \"links\" entry for each word in \"targets\". Your team's words are: %s", err, strings.Join(myWords, ", "))},
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

// clueReply is the schema the spymaster model must produce. Note the absence
// of any count field: the model names the words it means, and the count is
// derived from that list, so a clue can never claim a number that disagrees
// with the words behind it.
type clueReply struct {
	Clue    string            `json:"clue"`
	Targets []string          `json:"targets"`
	Links   map[string]string `json:"links"`
}

// parseClueResponse validates the model's JSON reply against myWords and
// derives the clue count from len(targets). It returns the clue and a
// human-readable rendering of the per-word justifications.
//
// Every failure here is a hard reject — we never repair a malformed reply into
// a playable clue, because a repaired clue is one whose count no longer
// reflects words the model actually committed to.
func parseClueResponse(reply string, myWords []string, board []codenames.Card) (*codenames.Clue, string, error) {
	raw, err := extractJSONObject(reply)
	if err != nil {
		return nil, "", err
	}

	var cr clueReply
	if err := json.Unmarshal([]byte(raw), &cr); err != nil {
		return nil, "", fmt.Errorf("reply was not a valid JSON object: %w", err)
	}

	word := strings.ToLower(strings.TrimSpace(cr.Clue))
	if word == "" {
		return nil, "", errors.New(`"clue" was empty`)
	}
	if strings.ContainsAny(word, " \t-_") {
		return nil, "", fmt.Errorf("clue %q must be a single word", word)
	}
	if conflict, ok := codenames.ConflictingBoardWord(word, board); ok {
		return nil, "", fmt.Errorf("clue %q is or contains the board word %q; clues may never be words on the board", word, conflict)
	}

	if len(cr.Targets) == 0 {
		return nil, "", errors.New(`"targets" was empty; list the words your clue points to`)
	}

	// Canonicalize each target to the board's own spelling, rejecting anything
	// that isn't one of this team's unrevealed words. Without this the derived
	// count could include a word the team doesn't even own.
	byLower := make(map[string]string, len(myWords))
	for _, w := range myWords {
		byLower[strings.ToLower(strings.TrimSpace(w))] = w
	}

	targets := make([]string, 0, len(cr.Targets))
	seen := make(map[string]bool, len(cr.Targets))
	for _, t := range cr.Targets {
		key := strings.ToLower(strings.TrimSpace(t))
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

	// The hard reject: links must cover targets exactly. One justification per
	// word, no spares — a model that can't name why a word is reached doesn't
	// get to count it.
	if len(cr.Links) != len(targets) {
		return nil, "", fmt.Errorf("got %d \"links\" entries for %d targets; provide exactly one per target", len(cr.Links), len(targets))
	}
	links := make(map[string]string, len(cr.Links))
	for k, v := range cr.Links {
		key := strings.ToLower(strings.TrimSpace(k))
		if !seen[key] {
			return nil, "", fmt.Errorf("\"links\" has an entry for %q, which is not in \"targets\"", k)
		}
		if strings.TrimSpace(v) == "" {
			return nil, "", fmt.Errorf("\"links\" entry for %q was empty; say why the clue points there", k)
		}
		links[key] = strings.TrimSpace(v)
	}

	// len(links) == len(targets) and every key is a distinct target, so the
	// two sets are now equal.

	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		parts = append(parts, fmt.Sprintf("%s: %s", t, links[strings.ToLower(t)]))
	}

	return &codenames.Clue{Word: word, Count: len(targets)}, strings.Join(parts, "; "), nil
}

// extractJSONObject pulls the outermost {...} out of a reply. Ollama's JSON
// mode makes this nearly always a no-op, but models still occasionally fence
// the object in ```json blocks.
func extractJSONObject(reply string) (string, error) {
	start := strings.IndexByte(reply, '{')
	end := strings.LastIndexByte(reply, '}')
	if start < 0 || end <= start {
		return "", errors.New("reply contained no JSON object")
	}
	return reply[start : end+1], nil
}

// Guess implements codenames.Operative.
func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	return ai.GuessOrPass(b, c, true /* mustGuess */)
}

// GuessOrPass is like Guess, but when mustGuess is false it may return
// codenames.PassGuess to end the turn instead of risking a bad guess.
func (ai *AI) GuessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, error) {
	guess, _, err := ai.guessOrPass(b, c, mustGuess)
	return guess, err
}

// GuessOrPassWithReasoning is like GuessOrPass, but also returns a
// human-readable explanation of why the guess (or pass) was chosen.
func (ai *AI) GuessOrPassWithReasoning(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, string, error) {
	return ai.guessOrPass(b, c, mustGuess)
}

func (ai *AI) guessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, string, error) {
	var unrevealed []string
	for _, card := range b.Cards {
		if !card.Revealed {
			unrevealed = append(unrevealed, card.Codename)
		}
	}

	system := `You are a human playing as a Codenames operative. Given a one-word clue and a count from your spymaster, you must guess which word on the board the clue refers to.

Rules:
- You must pick exactly ONE word from the board.
- Think about what the spymaster was *intending* with the clue, not just raw word similarity.
- Prioritize the most obvious, intuitive connection — the one a person would see first.
- If several words seem to fit, pick the safest, most direct one.
- Respond with EXACTLY two lines: the single board word on the first line, then "REASON: <one short sentence>" on the second line explaining your choice. No other text.`

	if !mustGuess {
		system += `
- You have already made at least one guess for this clue. If none of the remaining words connect well to the clue, or every candidate feels too risky, respond with PASS on the first line (followed by a REASON line) to stop guessing and end your turn.`
	}

	prompt := fmt.Sprintf(`The clue is: %s %d

Words on the board: %s

Your guess:`, c.Word, c.Count, strings.Join(unrevealed, ", "))

	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	}

	// All retries share one timeout budget, so a slow model can't multiply
	// the wait by re-trying — worst case is still one bounded wait, after
	// which we fall back to a random legal guess. Guesses get a tighter
	// budget than clues so they comfortably fit under the shorter
	// human-think delay ceiling applied to guesses (see humanThinkDelay in
	// cmd/ai-server/server.go).
	guessBudget := min(ai.timeout, 12*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), guessBudget)
	defer cancel()

	// Try up to 3 times to get a valid board word.
	for attempt := range 3 {
		reply, err := ai.chat(ctx, messages, "")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[LLM Operative] clue=%q timed out after attempt=%d, falling back", c.Word, attempt+1)
				return "", "", nil
			}
			return "", "", fmt.Errorf("llm chat: %w", err)
		}

		log.Printf("[LLM Operative] clue=%q, attempt=%d, raw response: %q", c.Word, attempt+1, reply)

		guess := parseGuessResponse(reply, unrevealed)
		if guess != "" {
			reasoning := extractReason(reply)
			log.Printf("[LLM Operative] guess: %q (reason: %s)", guess, reasoning)
			return guess, reasoning, nil
		}

		if !mustGuess && isPassResponse(reply) {
			reasoning := extractReason(reply)
			log.Printf("[LLM Operative] passing on clue %q (reason: %s)", c.Word, reasoning)
			return codenames.PassGuess, reasoning, nil
		}

		// Ask the model to try again with the board words emphasized.
		messages = append(messages,
			chatMessage{Role: "assistant", Content: reply},
			chatMessage{Role: "user", Content: fmt.Sprintf("That word is not on the board. You MUST pick from: %s", strings.Join(unrevealed, ", "))},
		)
	}

	// All retries failed — return empty to trigger random guess fallback.
	log.Printf("[LLM Operative] all retries failed for clue %q, falling back", c.Word)
	return "", "", nil
}

// isPassResponse reports whether the LLM's reply is a pass. Board-word
// matches are checked first by the caller, so a board word named "pass" still
// resolves to a guess.
func isPassResponse(reply string) bool {
	firstLine := strings.Split(strings.TrimSpace(reply), "\n")[0]
	firstLine = strings.Trim(strings.TrimSpace(firstLine), `*."'`)
	return strings.EqualFold(firstLine, "pass")
}

// parseGuessResponse finds the best matching board word from the LLM's response.
func parseGuessResponse(reply string, boardWords []string) string {
	reply = strings.TrimSpace(reply)

	// First, try exact match (case-insensitive) against board words.
	for _, w := range boardWords {
		if strings.EqualFold(reply, w) {
			return w
		}
	}

	// The model might have added extra text. Check if any board word appears
	// in the first line of the response.
	firstLine := strings.Split(reply, "\n")[0]
	firstLine = strings.ToLower(strings.TrimSpace(firstLine))
	for _, w := range boardWords {
		if strings.EqualFold(firstLine, w) {
			return w
		}
	}

	// Fallback: find any board word contained in the response.
	lower := strings.ToLower(reply)
	for _, w := range boardWords {
		if strings.Contains(lower, strings.ToLower(w)) {
			return w
		}
	}

	// No valid board word found.
	return ""
}

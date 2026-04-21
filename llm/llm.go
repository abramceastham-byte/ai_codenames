// Package llm provides Codenames Spymaster and Operative implementations
// backed by a local LLM via the Ollama API.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/bcspragu/Codenames/codenames"
)

// AI implements codenames.Spymaster and codenames.Operative using a local
// Ollama model.
type AI struct {
	endpoint string // e.g. "http://localhost:11434"
	model    string // e.g. "llama3"
}

func New(endpoint, model string) *AI {
	return &AI{endpoint: endpoint, model: model}
}

// Ollama chat API types

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

func (ai *AI) chat(messages []chatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    ai.model,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(ai.endpoint+"/api/chat", "application/json", bytes.NewReader(body))
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

	system := `You are an expert Codenames spymaster. You must give a single-word clue and a count of how many of your team's words it relates to.

Rules:
- Your clue must be a SINGLE word (no spaces, no hyphens, no proper nouns).
- Your clue cannot be any word on the board or a variant/substring of a board word.
- You MUST avoid clues that relate to the assassin word — guessing it loses the game instantly.
- You should avoid clues that relate to opponent words or bystanders.
- Try to link as many of your words as possible, but only if the connection is strong.

Respond with EXACTLY one line in the format: WORD COUNT
For example: OCEAN 3`

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

	reply, err := ai.chat([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	log.Printf("[LLM Spymaster] raw response: %q", reply)

	clue, err := parseClueResponse(reply)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM clue %q: %w", reply, err)
	}

	log.Printf("[LLM Spymaster] clue: %s %d", clue.Word, clue.Count)
	return clue, nil
}

// parseClueResponse extracts a "WORD COUNT" clue from the LLM's response.
// It tries the last line first (in case the model adds preamble), then the first line.
func parseClueResponse(reply string) (*codenames.Clue, error) {
	lines := strings.Split(strings.TrimSpace(reply), "\n")

	// Try each line, last first, looking for "WORD NUMBER" pattern.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		// Strip common prefixes the model might add
		line = strings.TrimPrefix(line, "Clue: ")
		line = strings.TrimPrefix(line, "clue: ")
		line = strings.TrimPrefix(line, "**")
		line = strings.TrimSuffix(line, "**")
		line = strings.TrimSpace(line)

		parts := strings.Fields(line)
		if len(parts) == 2 {
			count, err := strconv.Atoi(parts[1])
			if err == nil && count >= 1 {
				word := strings.ToLower(parts[0])
				return &codenames.Clue{Word: word, Count: count}, nil
			}
		}
	}

	return nil, fmt.Errorf("could not find WORD COUNT pattern in response")
}

// Guess implements codenames.Operative.
func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	var unrevealed []string
	for _, card := range b.Cards {
		if !card.Revealed {
			unrevealed = append(unrevealed, card.Codename)
		}
	}

	system := `You are an expert Codenames operative. Given a one-word clue and a count from your spymaster, you must guess which word on the board the clue refers to.

Rules:
- You must pick exactly ONE word from the board.
- Choose the word most strongly associated with the clue.
- Respond with ONLY the single board word, nothing else. No explanation, no punctuation.`

	prompt := fmt.Sprintf(`The clue is: %s %d

Words on the board: %s

Your guess:`, c.Word, c.Count, strings.Join(unrevealed, ", "))

	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	}

	// Try up to 3 times to get a valid board word.
	for attempt := range 3 {
		reply, err := ai.chat(messages)
		if err != nil {
			return "", fmt.Errorf("llm chat: %w", err)
		}

		log.Printf("[LLM Operative] clue=%q, attempt=%d, raw response: %q", c.Word, attempt+1, reply)

		guess := parseGuessResponse(reply, unrevealed)
		if guess != "" {
			log.Printf("[LLM Operative] guess: %q", guess)
			return guess, nil
		}

		// Ask the model to try again with the board words emphasized.
		messages = append(messages,
			chatMessage{Role: "assistant", Content: reply},
			chatMessage{Role: "user", Content: fmt.Sprintf("That word is not on the board. You MUST pick from: %s", strings.Join(unrevealed, ", "))},
		)
	}

	// All retries failed — return empty to trigger random guess fallback.
	log.Printf("[LLM Operative] all retries failed for clue %q, falling back", c.Word)
	return "", nil
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

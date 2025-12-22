// Package embedding provides an AI implementation using a remote embedding service.
package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

// AI implements the codenames.Spymaster and codenames.Operative interfaces
// using a remote embedding service for similarity computation.
type AI struct {
	endpoint string
	client   *http.Client
}

// New creates a new embedding AI client that connects to the given endpoint.
func New(endpoint string) *AI {
	return &AI{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// GiveClue implements codenames.Spymaster. Currently returns a placeholder
// since clue generation via embeddings is complex and not yet implemented.
func (ai *AI) GiveClue(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, error) {
	// Placeholder: clue generation is complex and left for future work
	return &codenames.Clue{Word: "placeholder", Count: 1}, nil
}

// Guess implements codenames.Operative. It computes similarity between the clue
// and all unrevealed cards, returning the most similar one.
func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	unused := codenames.Unused(b.Cards)
	if len(unused) == 0 {
		return "", nil
	}

	// Build candidate list with mapping back to original codenames
	type candidate struct {
		codename    string // Original codename on the card
		transformed string // Transformed version for embedding lookup
	}
	var candidates []candidate
	for _, card := range unused {
		// Handle underscores like w2v does: try both without underscore
		// and with space replacement
		if strings.Contains(card.Codename, "_") {
			candidates = append(candidates, candidate{
				codename:    card.Codename,
				transformed: strings.ReplaceAll(card.Codename, "_", ""),
			})
			candidates = append(candidates, candidate{
				codename:    card.Codename,
				transformed: strings.ReplaceAll(card.Codename, "_", " "),
			})
		} else {
			candidates = append(candidates, candidate{
				codename:    card.Codename,
				transformed: card.Codename,
			})
		}
	}

	// Extract just the transformed words for the API call
	words := make([]string, len(candidates))
	for i, c := range candidates {
		words[i] = c.transformed
	}

	scores, err := ai.similarity(c.Word, words)
	if err != nil {
		return "", fmt.Errorf("failed to get similarity: %w", err)
	}

	if len(scores) == 0 {
		return "", nil
	}

	// Find the original codename for the highest scoring transformed word
	bestWord := scores[0].Word
	for _, c := range candidates {
		if c.transformed == bestWord {
			return c.codename, nil
		}
	}

	// Fallback: return the word as-is (shouldn't happen)
	return bestWord, nil
}

type similarityRequest struct {
	Clue       string   `json:"clue"`
	Candidates []string `json:"candidates"`
}

type wordScore struct {
	Word       string  `json:"word"`
	Similarity float32 `json:"similarity"`
}

type similarityResponse struct {
	Scores []wordScore `json:"scores"`
}

func (ai *AI) similarity(clue string, candidates []string) ([]wordScore, error) {
	req := similarityRequest{
		Clue:       strings.ToLower(clue),
		Candidates: candidates,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := ai.client.Post(
		ai.endpoint+"/similarity",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call embedding service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned status %d", resp.StatusCode)
	}

	var result similarityResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Scores, nil
}

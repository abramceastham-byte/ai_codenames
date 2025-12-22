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

// GiveClue implements codenames.Spymaster. It finds a clue word that is
// semantically similar to our team's words and dissimilar to opponent/neutral/assassin words.
func (ai *AI) GiveClue(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, error) {
	// Get our team's unrevealed target words
	targets := codenames.Unrevealed(codenames.Targets(b.Cards, agent))
	if len(targets) == 0 {
		return &codenames.Clue{Word: "done", Count: 0}, nil
	}

	// Get words to avoid: opponent, bystanders, assassin
	var avoid []codenames.Card
	for _, a := range []codenames.Agent{codenames.Bystander, codenames.Assassin} {
		avoid = append(avoid, codenames.Unrevealed(codenames.Targets(b.Cards, a))...)
	}
	// Add opponent's cards
	opponent := opponentAgent(agent)
	if opponent != codenames.UnknownAgent {
		avoid = append(avoid, codenames.Unrevealed(codenames.Targets(b.Cards, opponent))...)
	}

	// Extract words from cards
	targetWords := cardWords(targets)
	avoidWords := cardWords(avoid)
	boardWords := allBoardWords(b)

	// Call the embedding service
	suggestions, err := ai.suggestClues(targetWords, avoidWords, boardWords, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get clue suggestions: %w", err)
	}

	if len(suggestions) == 0 {
		// Fallback if no suggestions
		return &codenames.Clue{Word: "guess", Count: 1}, nil
	}

	// Return the best suggestion with count=1
	// Future: could analyze target_scores to determine optimal count
	return &codenames.Clue{Word: suggestions[0].Clue, Count: 1}, nil
}

func opponentAgent(agent codenames.Agent) codenames.Agent {
	switch agent {
	case codenames.RedAgent:
		return codenames.BlueAgent
	case codenames.BlueAgent:
		return codenames.RedAgent
	default:
		return codenames.UnknownAgent
	}
}

func cardWords(cards []codenames.Card) []string {
	words := make([]string, len(cards))
	for i, c := range cards {
		words[i] = c.Codename
	}
	return words
}

func allBoardWords(b *codenames.Board) []string {
	words := make([]string, len(b.Cards))
	for i, c := range b.Cards {
		words[i] = c.Codename
	}
	return words
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

type suggestCluesRequest struct {
	Targets        []string `json:"targets"`
	Avoid          []string `json:"avoid"`
	BoardWords     []string `json:"board_words"`
	MaxSuggestions int      `json:"max_suggestions"`
}

type clueSuggestion struct {
	Clue         string    `json:"clue"`
	Score        float32   `json:"score"`
	TargetScores []float32 `json:"target_scores"`
}

type suggestCluesResponse struct {
	Suggestions []clueSuggestion `json:"suggestions"`
}

func (ai *AI) suggestClues(targets, avoid, boardWords []string, maxSuggestions int) ([]clueSuggestion, error) {
	req := suggestCluesRequest{
		Targets:        targets,
		Avoid:          avoid,
		BoardWords:     boardWords,
		MaxSuggestions: maxSuggestions,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := ai.client.Post(
		ai.endpoint+"/suggest-clues",
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

	var result suggestCluesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Suggestions, nil
}

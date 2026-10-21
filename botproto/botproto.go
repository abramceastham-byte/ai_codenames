// Package botproto defines the JSON protocol between the game engine and
// tournament bots. Bots are separate executables: for every move the engine
// starts the bot, writes one Request to its stdin, and reads one response
// (SpymasterResponse or OperativeResponse) from its stdout.
//
// The same protocol is described as JSON Schema in schema.json and for
// students in BOT_GUIDE.md. Keep all three in sync.
package botproto

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ProtocolVersion is sent with every request. Bump it on breaking changes.
const ProtocolVersion = 1

const (
	// DefaultMoveTimeout is how long a bot has to answer one request.
	DefaultMoveTimeout = 10 * time.Second
	// MaxResponseBytes caps how much of a bot's stdout the engine reads.
	MaxResponseBytes = 64 * 1024
	// MinClueCount and MaxClueCount bound the count in a clue.
	MinClueCount = 1
	MaxClueCount = 9
	// MaxClueLength bounds the length of a clue word.
	MaxClueLength = 30
	// MaxTurns ends a game that both teams keep stalling. Each clue (or
	// forfeited clue) is one turn.
	MaxTurns = 50
)

// Role is which seat a request is for.
type Role string

const (
	RoleSpymaster Role = "spymaster"
	RoleOperative Role = "operative"
)

// Team is a team color.
type Team string

const (
	TeamRed  Team = "red"
	TeamBlue Team = "blue"
)

// CardType is the hidden identity of a card.
type CardType string

const (
	CardRed       CardType = "red"
	CardBlue      CardType = "blue"
	CardBystander CardType = "bystander"
	CardAssassin  CardType = "assassin"
)

// Card is one board position. For operatives, Type is omitted (empty) on
// cards that haven't been revealed yet.
type Card struct {
	Word       string   `json:"word"`
	Type       CardType `json:"type,omitempty"`
	Revealed   bool     `json:"revealed"`
	RevealedBy Team     `json:"revealed_by,omitempty"`
}

// Clue is a spymaster's clue.
type Clue struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

// Guess is one guess made during a turn and what it turned out to be.
type Guess struct {
	Word   string   `json:"word"`
	Result CardType `json:"result"`
}

// TurnEnd says why a turn ended.
type TurnEnd string

const (
	// EndWrongCard: the operative revealed a card that wasn't their team's.
	EndWrongCard TurnEnd = "wrong_card"
	// EndOutOfGuesses: the operative used all count+1 guesses.
	EndOutOfGuesses TurnEnd = "out_of_guesses"
	// EndStopped: the operative's guess list ran out with guesses to spare.
	EndStopped TurnEnd = "stopped"
	// EndGameOver: the turn ended the game.
	EndGameOver TurnEnd = "game_over"
	// EndSpymasterForfeit: the spymaster timed out, crashed, sent bad JSON,
	// or gave an illegal clue. Clue is null and no guesses were made.
	EndSpymasterForfeit TurnEnd = "spymaster_forfeit"
	// EndOperativeForfeit: the operative timed out, crashed, sent bad JSON,
	// sent no guesses, or guessed an illegal word. Guesses before the bad one
	// still count.
	EndOperativeForfeit TurnEnd = "operative_forfeit"
)

// Turn is one completed turn in the game history.
type Turn struct {
	Team    Team    `json:"team"`
	Clue    *Clue   `json:"clue"`
	Guesses []Guess `json:"guesses"`
	End     TurnEnd `json:"end"`
}

// Request is what the engine writes to a bot's stdin. Clue and
// GuessesAllowed are only set for operatives.
type Request struct {
	ProtocolVersion int    `json:"protocol_version"`
	Role            Role   `json:"role"`
	Team            Team   `json:"team"`
	Board           []Card `json:"board"`
	History         []Turn `json:"history"`

	Clue           *Clue `json:"clue,omitempty"`
	GuessesAllowed int   `json:"guesses_allowed,omitempty"`
}

// SpymasterResponse is what a spymaster writes to stdout.
type SpymasterResponse struct {
	Clue  string `json:"clue"`
	Count int    `json:"count"`
}

// OperativeResponse is what an operative writes to stdout. Guesses are tried
// in order, and there must be at least one.
type OperativeResponse struct {
	Guesses []string `json:"guesses"`
}

// ValidateClue reports why a clue is illegal for the given board, or nil if
// it's legal. Words are compared case-insensitively.
func ValidateClue(resp SpymasterResponse, board []Card) error {
	word := strings.ToLower(strings.TrimSpace(resp.Clue))
	if word == "" {
		return fmt.Errorf("clue is empty")
	}
	if len(word) > MaxClueLength {
		return fmt.Errorf("clue %q is longer than %d characters", resp.Clue, MaxClueLength)
	}
	for _, r := range word {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return fmt.Errorf("clue %q must be a single word of letters a-z", resp.Clue)
		}
	}
	if resp.Count < MinClueCount || resp.Count > MaxClueCount {
		return fmt.Errorf("count %d must be between %d and %d", resp.Count, MinClueCount, MaxClueCount)
	}
	for _, c := range board {
		if c.Revealed {
			continue
		}
		bw := strings.ToLower(c.Word)
		if strings.Contains(bw, word) || strings.Contains(word, bw) {
			return fmt.Errorf("clue %q overlaps board word %q", resp.Clue, c.Word)
		}
	}
	return nil
}

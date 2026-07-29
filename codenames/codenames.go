// Package codenames contains all of our domain types, and an interface for
// databases to implement, which should really live in the `web` package, but
// I wrote a lot of this before I understood how to properly structure these
// things.
package codenames

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	// Rows is the number of rows of cards in Codenames.
	Rows = 5
	// Columns is the number of columns of cards in Codenames.
	Columns = 5
	// Size is the total number of cards on a Codenames board.
	Size = Rows * Columns
)

type Spymaster interface {
	// GiveClue takes in a board and returns a clue for players to guess with.
	GiveClue(*Board, Agent) (*Clue, error)
}

type Operative interface {
	// Guess takes in a board and a clue and returns the guessed Codename from
	// the board.
	Guess(*Board, *Clue) (string, error)
}

// PassGuess is a sentinel guess value an AI Operative can return to decline
// guessing and end its team's turn. It's distinct from returning an empty
// string, which signals "no guess could be made" and triggers fallbacks. The
// angle brackets guarantee it can never collide with a board word.
const PassGuess = "<pass>"

// Board contains all of the information about a game of Codenames.
type Board struct {
	// Cards is a list of the 25 words on the board. The zeroth card corresponds
	// to the top-left, the fourth to the top-right, and the twenty-fourth to the
	// bottom-right.
	Cards []Card `json:"cards"`
}

func (b *Board) Clone() *Board {
	if b == nil {
		return nil
	}

	cards := make([]Card, len(b.Cards))
	copy(cards, b.Cards)

	return &Board{Cards: cards}
}

// Clue is a word and a count from the Spymaster.
type Clue struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

func (c *Clue) String() string {
	return c.Word + " " + strconv.Itoa(c.Count)
}

// Codename is a single game card, and its corresponding affiliation.
type Card struct {
	// Codename is the word on the card, the "codename" of the agent.
	Codename string `json:"codeword"`
	// Agent is the type of the card, or UnknownAgent if the player doesn't yet
	// know the affiliation.
	Agent Agent `json:"agent"`
	// Revealed is true if the card has been guessed and the identity has been
	// shown to operatives.
	Revealed bool `json:"revealed"`
	// Revealed by is set to the team that chose this card to turnover. This is
	// set to NoTeam unless Revealed is true.
	RevealedBy Team `json:"revealed_by"`
}

// Agent is the affiliation of a codename.
type Agent int

func (a Agent) String() string {
	switch a {
	case UnknownAgent:
		return "Agent Status Unknown"
	case RedAgent:
		return "Red Agent"
	case BlueAgent:
		return "Blue Agent"
	case Bystander:
		return "Bystander"
	case Assassin:
		return "Assassin"
	}
	return ""
}

const (
	// UnknownAgent means we don't know who the codename belongs to.
	UnknownAgent Agent = iota
	// RedAgent means the codename belongs to an agent on the red team.
	RedAgent
	// BlueAgent means the codename belongs to an agent on the blue team.
	BlueAgent
	// Bystander means the codename doesn't belong to an agent.
	Bystander
	// Assassin means the codename belongs to the assassin.
	Assassin
)

type Team string

func (t Team) String() string {
	switch t {
	case RedTeam:
		return "Red Team"
	case BlueTeam:
		return "Blue Team"
	}
	return ""
}

const (
	// NoTeam is an error case.
	NoTeam   = Team("")
	RedTeam  = Team("RED")
	BlueTeam = Team("BLUE")
)

func ToTeam(team string) (Team, bool) {
	switch team {
	case "RED":
		return RedTeam, true
	case "BLUE":
		return BlueTeam, true
	default:
		return NoTeam, false
	}
}

// Unused returns a list of cards that haven't been assigned an Agent type yet.
func Unused(cards []Card) []Card {
	return Targets(cards, UnknownAgent)
}

// Targets returns a list of cards that have been assigned to the given Agent
// type.
func Targets(cards []Card, agent Agent) []Card {
	var out []Card
	for _, card := range cards {
		if card.Agent == agent {
			out = append(out, card)
		}
	}
	return out
}

// Unrevealed returns a list of cards that haven't been turned over yet.
func Unrevealed(cards []Card) []Card {
	var out []Card
	for _, card := range cards {
		if !card.Revealed {
			out = append(out, card)
		}
	}
	return out
}

// ConflictingBoardWord reports the board word that a clue is an illegal form
// of, if any. A clue may not be a board word or an inflected/derived form of
// one — "king", "kings", "kingdom" and "kingly" are all illegal when "king" is
// on the board.
//
// The test is directional: the clue must share a *root* with the board word,
// not merely contain it. Plain containment is the wrong primitive — it would
// block "bring" for RING, "cover" for OVER and "start" for ART, none of which
// give anything away. Position is the discriminator, not length.
//
// Revealed cards are ignored, per the official rules: once a card is covered,
// its word is back in play as a clue. Blocking them forever would be a
// systematic difference between AI and human spymasters, which matters when
// the point is measuring whether the two can be told apart.
//
// Comparison is case-insensitive and ignores separators, because clue casing
// differs by source: human clues arrive upper-cased, LLM clues lower-cased, and
// w2v clues carry the embedding model's own casing.
func ConflictingBoardWord(clue string, cards []Card) (string, bool) {
	clue = normalizeWord(clue)
	if clue == "" {
		return "", false
	}
	cards = Unrevealed(cards)

	// Exact matches win, so the reported word is the one the caller most
	// likely meant. Otherwise a clue like "police" would be reported against
	// "ice" purely because that card came first, even though "police" is
	// itself on the board.
	for _, card := range cards {
		if slices.Contains(cardForms(card.Codename), clue) {
			return card.Codename, true
		}
	}

	for _, card := range cards {
		for _, form := range cardForms(card.Codename) {
			if sharesRoot(clue, form) {
				return card.Codename, true
			}
		}
	}
	return "", false
}

// SuspectedCompound reports a board word that appears inside the clue without
// being a shared root — "airport" for AIR, "kingfisher" for KING, "iceberg"
// for ICE.
//
// This deliberately does NOT feed a rejection. Separating a real compound from
// a coincidence ("justice" and "office" both contain "ice") needs a wordlist
// check on the remainder, which brings the false positives straight back. A
// clue wrongly refused mid-round is a visible defect in the instrument, so we
// record the near-miss for later analysis and let the turn proceed.
func SuspectedCompound(clue string, cards []Card) (string, bool) {
	clue = normalizeWord(clue)
	if clue == "" {
		return "", false
	}
	cards = Unrevealed(cards)

	for _, card := range cards {
		for _, form := range cardForms(card.Codename) {
			if len(form) < minRootLen || form == clue {
				continue
			}
			if strings.Contains(clue, form) && !sharesRoot(clue, form) {
				return card.Codename, true
			}
		}
	}
	return "", false
}

// cardForms returns the comparable forms of a board word: the whole thing with
// separators stripped, plus each token on its own. "ICE CREAM" has to be
// checked as "icecream" and as "ice" and "cream", or a clue of "icy" would
// sail past a board holding ICE CREAM.
func cardForms(codename string) []string {
	joined := normalizeWord(codename)
	if joined == "" {
		return nil
	}

	forms := []string{joined}
	for _, tok := range strings.FieldsFunc(strings.ToLower(codename), isWordSeparator) {
		if tok != "" && tok != joined {
			forms = append(forms, tok)
		}
	}
	return forms
}

func isWordSeparator(r rune) bool {
	return r == ' ' || r == '_' || r == '-' || r == '\t'
}

// minRootLen is the shortest word we'll treat as a root. Below this, only an
// exact match counts — a two-letter card can't meaningfully be inflected.
const minRootLen = 3

var suffixes = map[string]bool{
	"s": true, "es": true, "ed": true, "ing": true, "er": true, "ers": true,
	"est": true, "ies": true, "ied": true, "y": true, "ly": true,
	"dom": true, "ship": true, "hood": true, "ness": true, "ful": true,
	"less": true, "ish": true, "ist": true, "ism": true, "able": true,
}

// vowelInitial suffixes are the ones that trigger e-drop and consonant
// doubling in English orthography: ice -> icing, run -> running.
var vowelInitial = map[string]bool{
	"es": true, "ed": true, "ing": true, "er": true, "ers": true,
	"est": true, "ies": true, "ied": true, "y": true,
}

// sharesRoot reports whether two normalized words are forms of the same root.
// Either argument may be the root, so both directions are tried: "ice"/"icy"
// are the same length, and which one is the board word depends on the board.
func sharesRoot(a, b string) bool {
	if a == b {
		return true
	}
	return isDerivedForm(a, b) || isDerivedForm(b, a)
}

// isDerivedForm reports whether derived is root plus a legal English suffix,
// allowing for e-drop, consonant doubling and y->i.
func isDerivedForm(derived, root string) bool {
	if len(root) < minRootLen || derived == root {
		return false
	}

	// match checks whether derived is stem + a legal suffix. vowelOnly
	// restricts it to the vowel-initial suffixes, which are the only ones that
	// license e-drop and consonant doubling — that's what keeps "one" -> "on"
	// + "ly" from swallowing "only" while still catching "ice" -> "ic" + "y".
	match := func(stem string, vowelOnly bool) bool {
		rest, ok := strings.CutPrefix(derived, stem)
		if !ok || !suffixes[rest] {
			return false
		}
		if vowelOnly && !vowelInitial[rest] {
			return false
		}
		if rest == "ly" && len(root) < 4 {
			// "ear" + "ly" = early, "one" + "ly" = only.
			return false
		}
		return true
	}

	switch {
	case match(root, false): // kings, kingdom, kingly
		return true
	case match(root+root[len(root)-1:], true): // running, bigger
		return true
	case strings.HasSuffix(root, "e") &&
		match(root[:len(root)-1], true): // icing, icy, danced
		return true
	case strings.HasSuffix(root, "y") &&
		match(root[:len(root)-1]+"i", false): // cities, spied
		return true
	}
	return false
}

// normalizeWord lower-cases a word and strips the separators that board words
// and clues use inconsistently, so "SEA_HORSE", "sea horse" and "seahorse" all
// compare equal.
func normalizeWord(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	return strings.NewReplacer(" ", "", "_", "", "-", "").Replace(w)
}

// Revealed takes in a fully-filled out Spymaster board, and returns a new
// board where the card Agent is only populated for revealed cards.
func Revealed(b *Board, status GameStatus) *Board {
	out := make([]Card, len(b.Cards))
	copy(out, b.Cards)
	for i, card := range b.Cards {
		if !card.Revealed && status != Finished {
			out[i].Agent = UnknownAgent
		}
	}
	return &Board{Cards: out}
}

// CloneBoard returns a deep copy of the given board.
func CloneBoard(b *Board) *Board {
	out := make([]Card, len(b.Cards))
	copy(out, b.Cards)
	return &Board{Cards: out}
}

func ParseClue(clue string) (*Clue, error) {
	ps := strings.Split(clue, " ")
	if len(ps) != 2 {
		return nil, fmt.Errorf("malformed clue %q", clue)
	}
	i, err := strconv.Atoi(ps[1])
	if err != nil {
		return nil, fmt.Errorf("malformed number of words in clue %q: %v", clue, err)
	}

	return &Clue{Word: ps[0], Count: i}, nil
}

package w2v

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/bcspragu/Codenames/codenames"

	"code.sajari.com/word2vec"
)

type AI struct {
	GloveModel      *word2vec.Model
	ConceptNetModel *word2vec.Model
}

// Init initializes the word2vec model.
func New(gloveFile, conceptNetFile string) (*AI, error) {
	gloveModel, err := loadModel(gloveFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load glove model: %w", err)
	}

	conceptNetModel, err := loadModel(conceptNetFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load conceptNet model: %w", err)
	}

	return &AI{
		GloveModel:      gloveModel,
		ConceptNetModel: conceptNetModel,
	}, nil
}

func loadModel(file string) (*word2vec.Model, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open model file %q: %w", file, err)
	}
	defer f.Close()

	model, err := word2vec.FromReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model file %q: %w", file, err)
	}
	return model, nil
}

func (ai *AI) GiveClue(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, error) {
	bestScore := float32(-1.0)
	clue := "???"

	clueableTargets := codenames.Unrevealed(codenames.Targets(b.Cards, agent))

	// TODO: Select N random permutations of 2, 3, and 4 clueable targets, find CosN matches for each, rank top matches for each of 2, 3, and 4, pick one.
	// Matches for 3 or 4 are necessarily going to be lower than 2, so we should have some sliding weighting that would prefer a 4 to a 3 to a 2 even if it was worse, but only slightly so.

	for _, word := range toWordList(codenames.Unrevealed(codenames.Targets(b.Cards, agent))) {
		expr := word2vec.Expr{}
		expr.Add(1, word)
		matches, err := ai.ConceptNetModel.CosN(expr, 50)
		if errors.Is(err, word2vec.NotFoundError{}) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load similar words: %w", err)
		}

		for _, match := range matches {
			if tooCloseToBoardWord(match.Word, b) {
				continue
			}
			if match.Score > bestScore {
				bestScore = match.Score
				clue = match.Word
			}
		}
	}

	return &codenames.Clue{Word: clue, Count: 1}, nil
}

func tooCloseToBoardWord(clue string, b *codenames.Board) bool {
	for _, card := range b.Cards {
		if strings.Contains(clue, card.Codename) || strings.Contains(card.Codename, clue) {
			return true
		}
	}
	return false
}

func toWordList(targets []codenames.Card) []string {
	var available []string
	for _, c := range targets {
		// Some cards contain underscores, which makes them unlikely to appear in
		// the model corpus. So what we do is we try to insert two copies of the
		// word, one with the underscore removed, and one with the underscore
		// replaced with a space. The idea is that hopefully one of these appears
		// in the source corpus.
		if strings.Contains(c.Codename, "_") {
			available = append(available, strings.ReplaceAll(c.Codename, "_", ""))
			available = append(available, strings.ReplaceAll(c.Codename, "_", " "))
		} else {
			available = append(available, c.Codename)
		}
	}
	return available
}

func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	type pair struct {
		Word       string
		Similarity float32
	}

	var pairs []pair
	for _, word := range toWordList(codenames.Unused(b.Cards)) {
		sim, err := ai.similarity(c.Word, word)
		if errors.Is(err, word2vec.NotFoundError{}) {
			continue
		}
		if err != nil {
			log.Printf("failed to get similarity of %q and %q: %v", c.Word, word, err)
			continue
		}

		pairs = append(pairs, pair{
			Word:       word,
			Similarity: sim,
		})
	}

	// Sort the board words most similar -> least similar.
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Similarity > pairs[j].Similarity
	})

	// This is a crutch for when the player enters a word that isn't in the model.
	if len(pairs) == 0 {
		return "", nil
	}
	return pairs[0].Word, nil
}

// Similarity returns a value from 0 to 1, that is the similarity of the two
// input words.
func (ai *AI) similarity(a, b string) (float32, error) {
	s, err := ai.GloveModel.Cos(exp(strings.ToLower(a)), exp(strings.ToLower(b)))
	if err != nil {
		return 0.0, fmt.Errorf("failed to determine similarity: %w", err)
	}
	return s, nil
}

func exp(w string) word2vec.Expr {
	expr := word2vec.Expr{}
	expr.Add(1, w)
	return expr
}

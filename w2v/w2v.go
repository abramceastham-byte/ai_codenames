package w2v

import (
	"bufio"
	"errors"
	"fmt"
	"iter"
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/bcspragu/Codenames/codenames"

	"code.sajari.com/word2vec"
)

type AI struct {
	GloveModel      *word2vec.Model
	ConceptNetModel *word2vec.Model
	CommonWordlist  []string
}

// Init initializes the word2vec model.
func New(gloveFile, conceptNetFile, commonWordlistFile string) (*AI, error) {
	gloveModel, err := loadModel(gloveFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load glove model: %w", err)
	}

	conceptNetModel, err := loadModel(conceptNetFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load conceptNet model: %w", err)
	}

	var wordlist []string
	f, err := os.Open(commonWordlistFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open common wordlist: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		wordlist = append(wordlist, sc.Text())
	}

	return &AI{
		GloveModel:      gloveModel,
		ConceptNetModel: conceptNetModel,
		CommonWordlist:  wordlist,
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

// clueCandidate represents a potential clue with its score and target count.
type clueCandidate struct {
	word       string
	rawScore   float32
	count      int
	finalScore float32
	combo      []string
	gloveScore float32
}

// pair is a board word and its similarity score to a clue, used when ranking
// guesses.
type pair struct {
	Word       string
	Similarity float32
	// Agent is the card's true affiliation. Populated whenever the caller has
	// full board visibility (the spymaster's view); left UnknownAgent when
	// ranked from the operative's own (redacted) board, since that's all the
	// operative legitimately gets.
	Agent codenames.Agent
}

// formatClueReasoning summarizes the top-ranked clue candidates (winner plus
// runners-up) into a human-readable explanation of why a clue was chosen.
func formatClueReasoning(candidates []clueCandidate) string {
	n := 3
	if len(candidates) < n {
		n = len(candidates)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		c := candidates[i]
		word := strings.ReplaceAll(c.word, "_", " ")
		if i == 0 {
			fmt.Fprintf(&sb, "Picked %q (count=%d) targeting %v — raw=%.3f final=%.3f.", word, c.count, c.combo, c.rawScore, c.finalScore)
		} else {
			fmt.Fprintf(&sb, " Runner-up: %q targeting %v final=%.3f.", word, c.combo, c.finalScore)
		}
	}
	return sb.String()
}

// formatGuessReasoning summarizes the top-ranked guess candidates (winner
// plus runners-up) into a human-readable explanation of why a guess was
// chosen.
func formatGuessReasoning(pairs []pair, clueWord string) string {
	n := 3
	if len(pairs) < n {
		n = len(pairs)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		p := pairs[i]
		if i == 0 {
			fmt.Fprintf(&sb, "Picked %q (similarity=%.3f) for clue %q.", p.Word, p.Similarity, clueWord)
		} else {
			fmt.Fprintf(&sb, " Runner-up: %q (%.3f).", p.Word, p.Similarity)
		}
	}
	return sb.String()
}

// countBonus returns a multiplier bonus for clues that target more words.
// Higher counts get a bonus to prefer them over lower counts with similar raw scores.
func countBonus(count int) float32 {
	switch count {
	case 4:
		return 1.10
	case 3:
		return 1.08
	case 2:
		return 1.06
	default:
		return 1
	}
}

func multiWordPenalty(inp string) float32 {
	cnt := strings.Count(inp, " ") + strings.Count(inp, "_")
	switch cnt {
	case 0:
		return 1
	case 1:
		return 0.95
	case 2:
		return 0.9
	default:
		return 0.8
	}
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

type embedMapping struct {
	// Codenames word -> word in the embedding
	toEmbedWord map[string]string
}

func wordVariants(inp string) iter.Seq[string] {
	return func(yield func(string) bool) {
		inp = strings.ToLower(inp)
		if !yield(inp) {
			return
		}
		if !yield(strings.ReplaceAll(inp, "_", " ")) {
			return
		}
		if !yield(strings.ReplaceAll(inp, "_", "")) {
			return
		}
		if !yield(strings.ReplaceAll(inp, " ", "_")) {
			return
		}
		if !yield(strings.ReplaceAll(inp, " ", "")) {
			return
		}
		log.Printf("no valid word variants found in model for %q", inp)
	}
}

func makeEmbedMapping(model *word2vec.Model, targets ...[]codenames.Card) embedMapping {
	toEmbedWord := make(map[string]string)

	for _, targetList := range targets {
		for _, card := range targetList {
			for variant := range wordVariants(card.Codename) {
				expr := word2vec.Expr{}
				expr.Add(1, variant)
				_, err := model.Eval(expr)
				if err == nil {
					// Means this variant is in the model
					toEmbedWord[card.Codename] = variant
					break
				}
			}
		}
	}

	return embedMapping{
		toEmbedWord: toEmbedWord,
	}
}

func (em embedMapping) filterToValid(inp []codenames.Card) []codenames.Card {
	n := 0
	for _, c := range inp {
		if _, ok := em.toEmbedWord[c.Codename]; ok {
			inp[n] = c
			n++
		}
	}
	return inp[:n]
}

// Makes cards out of all the candidate clues + the combos they're matched on
func candidatesToCards(candidates []clueCandidate) []codenames.Card {
	out := make([]codenames.Card, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, codenames.Card{Codename: c.word})
		for _, cc := range c.combo {
			out = append(out, codenames.Card{Codename: cc})
		}
	}
	return out
}

const maxCombinations = 200
const maxCluesToConsider = 9

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
	clueableTargets := codenames.Unrevealed(codenames.Targets(b.Cards, agent))
	avoidTargets := codenames.Unrevealed(codenames.Targets(b.Cards, opponentAgent(agent)))
	assassinTargets := codenames.Targets(b.Cards, codenames.Assassin)

	embedMapping := makeEmbedMapping(ai.ConceptNetModel, clueableTargets, avoidTargets, assassinTargets)
	// Only evaluate words we found in our model
	clueableTargets = embedMapping.filterToValid(clueableTargets)
	avoidTargets = embedMapping.filterToValid(avoidTargets)
	assassinTargets = embedMapping.filterToValid(assassinTargets)

	// Since we try every combination, we want to not go too crazy. 9 choose 4 = 126, which is a reasonable upper bound.
	if len(clueableTargets) > maxCluesToConsider {
		clueableTargets = clueableTargets[:maxCluesToConsider]
	}

	var candidates []clueCandidate

	// Try combinations of sizes 1, 2, 3, and 4
	// Size 1 is really just an escape hatch if we can't come up with anything good.
	for _, size := range []int{4, 3, 2, 1} {
		if len(clueableTargets) < size {
			continue
		}

		combos := combinations(clueableTargets, size)
		// If there are too many combinations, sample randomly
		if len(combos) > maxCombinations {
			rand.Shuffle(len(combos), func(i, j int) {
				combos[i], combos[j] = combos[j], combos[i]
			})
			combos = combos[:maxCombinations]
		}

		for _, combo := range combos {
			expr := word2vec.Expr{}
			for _, word := range combo {
				expr.Add(1, embedMapping.toEmbedWord[word])
			}
			for _, card := range avoidTargets {
				expr.Add(-0.3, embedMapping.toEmbedWord[card.Codename])
			}
			for _, card := range assassinTargets {
				expr.Add(-1, embedMapping.toEmbedWord[card.Codename])
			}

			matches := ai.CosN(ai.ConceptNetModel, expr, 10)

			for _, match := range matches {
				if tooCloseToBoardWord(match.Word, b, combo) {
					continue
				}
				finalScore := match.Score * countBonus(size) * multiWordPenalty(match.Word)
				candidates = append(candidates, clueCandidate{
					word:       match.Word,
					rawScore:   match.Score,
					count:      size,
					finalScore: finalScore,
					combo:      combo,
				})
			}
		}
	}

	gloveMap := makeEmbedMapping(ai.GloveModel, candidatesToCards(candidates))

	if len(candidates) == 0 {
		return &codenames.Clue{Word: "???", Count: 1}, "no viable clue candidates found in the search space; defaulting to placeholder", nil
	}

	// Now populate the glove scores
	for i, c := range candidates {
		expr := word2vec.Expr{}
		for _, word := range c.combo {
			expr.Add(1, gloveMap.toEmbedWord[word])
		}
		gloveScore, err := ai.GloveModel.Cos(expr, exp(gloveMap.toEmbedWord[c.word]))
		if err != nil {
			log.Printf("couldn't get glove score, which shouldn't happen because we already checked: %v", err)
			continue
		}
		candidates[i].gloveScore = gloveScore
	}

	minGlove, maxGlove := candidates[0].gloveScore, candidates[0].gloveScore
	for _, c := range candidates {
		if c.gloveScore == 0 {
			continue
		}
		if c.gloveScore < minGlove {
			minGlove = c.gloveScore
		}
		if c.gloveScore > maxGlove {
			maxGlove = c.gloveScore
		}
	}

	// Update the final score based on GloVe's opinion
	for _, c := range candidates {
		if c.gloveScore == 0 {
			continue
		}
		coeff := (c.gloveScore - minGlove) / (maxGlove - minGlove)
		// Boost by up to 33% for good alignment
		c.finalScore *= (1 + coeff/3)
	}

	// Sort by final score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].finalScore > candidates[j].finalScore
	})

	z := 25
	if nn := len(candidates); nn < 25 {
		z = nn
	}

	for i := range z {
		log.Printf("Best candidate #%d: %q for %+v, raw score %f, %f", i, candidates[i].word, candidates[i].combo, candidates[i].rawScore, candidates[i].finalScore)
	}

	best := candidates[0]
	word := strings.ReplaceAll(best.word, "_", " ")
	log.Printf("My clue is %q, targetting %+v", word, best.combo)
	return &codenames.Clue{Word: word, Count: best.count}, formatClueReasoning(candidates), nil
}

// The difference between our CosN and (*word2vec.Model).CosN is that we only
// search against a common wordlist, which is much smaller than the entire
// wordlist that the model search goes over.
func (ai *AI) CosN(m *word2vec.Model, expToTest word2vec.Expr, n int) []word2vec.Match {
	r := make([]word2vec.Match, n)
	for _, w := range ai.CommonWordlist {
		score, err := m.Cos(expToTest, exp(w))
		if err != nil {
			log.Printf("[CosN] we got the 'word not found' error, which really shouldn't happen since we already filtered stuff: %v", err)
			continue
		}
		if r[n-1].Score > score {
			continue
		}
		p := word2vec.Match{Word: w, Score: score}
		r[n-1] = p
		for j := n - 2; j >= 0; j-- {
			if r[j].Score > p.Score {
				break
			}
			r[j], r[j+1] = p, r[j]
		}
	}
	return r
}

// combinations returns all combinations of size k from the input slice.
func combinations(input []codenames.Card, k int) [][]string {
	if k > len(input) {
		return nil
	}
	if k == 0 {
		return [][]string{{}}
	}

	var result [][]string
	var helper func(start int, current []string)
	helper = func(start int, current []string) {
		if len(current) == k {
			combo := make([]string, k)
			copy(combo, current)
			result = append(result, combo)
			return
		}
		for i := start; i < len(input); i++ {
			helper(i+1, append(current, input[i].Codename))
		}
	}
	helper(0, nil)
	return result
}

func tooCloseToBoardWord(clue string, b *codenames.Board, combo []string) bool {
	if _, ok := codenames.ConflictingBoardWord(clue, b.Cards); ok {
		return true
	}
	for _, target := range combo {
		if hammingDistance(clue, target) < 3 {
			return true
		}
	}

	return false
}

// hammingDistance returns the number of positions at which the corresponding
// characters differ. For strings of different lengths, the length difference
// is added to the distance.
func hammingDistance(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if len(a) > len(b) {
		a, b = b, a
	}
	dist := len(b) - len(a)
	for i := range a {
		if a[i] != b[i] {
			dist++
		}
	}
	return dist
}

// passThreshold is the minimum cosine similarity between the clue and the
// best remaining board word for the operative to risk another guess. Related
// ConceptNet words usually score 0.3+, unrelated ones under ~0.15.
const passThreshold = 0.15

func (ai *AI) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	return ai.GuessOrPass(b, c, true /* mustGuess */)
}

// GuessOrPass is like Guess, but when mustGuess is false it may return
// codenames.PassGuess to end the turn if even the best remaining word is a
// poor match for the clue.
func (ai *AI) GuessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, error) {
	guess, _, err := ai.guessOrPass(b, c, mustGuess)
	return guess, err
}

// GuessOrPassWithReasoning is like GuessOrPass, but also returns a
// human-readable explanation of why the guess (or pass) was chosen.
func (ai *AI) GuessOrPassWithReasoning(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, string, error) {
	return ai.guessOrPass(b, c, mustGuess)
}

// rankBoardWords ranks cards by ConceptNet similarity to clueWord, most
// similar first. Each returned pair's Agent mirrors the Agent already on the
// card passed in — so it carries real team info when called with a
// spymaster's full board, and UnknownAgent when called with an operative's
// redacted one. Shared by guessOrPass (operative, redacted board) and
// BonusGuessSignal (spymaster, full board) so the ranking logic itself is
// never duplicated.
func (ai *AI) rankBoardWords(cards []codenames.Card, clueWord string) []pair {
	embedMapping := makeEmbedMapping(ai.ConceptNetModel, cards, []codenames.Card{{Codename: clueWord}})
	cards = embedMapping.filterToValid(cards)

	var pairs []pair
	for _, card := range cards {
		sim, err := ai.similarity(embedMapping.toEmbedWord[clueWord], embedMapping.toEmbedWord[card.Codename])
		if errors.Is(err, &word2vec.NotFoundError{}) {
			continue
		}
		if err != nil {
			log.Printf("failed to get similarity of %q and %q: %v", clueWord, card.Codename, err)
			continue
		}

		pairs = append(pairs, pair{
			Word:       card.Codename,
			Similarity: sim,
			Agent:      card.Agent,
		})
	}

	// Sort the board words most similar -> least similar.
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Similarity > pairs[j].Similarity
	})
	return pairs
}

// BonusGuessSignal computes the two risk signals operative.AllowBonus needs
// — margin and assassinInTop3 — for a clue just given, using the spymaster's
// full, unredacted board.
//
// CALLER MUST PASS THE SPYMASTER'S FULL BOARD, NEVER THE OPERATIVE'S. The
// operative's own board view has every unrevealed card's Agent zeroed to
// UnknownAgent (codenames.Revealed), so calling this with that board would
// silently produce a meaningless margin (no card would ever match ourAgent)
// and assassinInTop3 always false — not an error, just quietly wrong, which
// is worse than a crash. This method exists specifically so that
// computation only ever happens where a full board is legitimately
// available (spymaster-side, at clue-give time), never inside the
// operative's own guess path.
//
// margin is the ConceptNet-similarity gap, for clueWord, between our team's
// best remaining unrevealed word and the best remaining non-team word
// (opponent, bystander, or assassin) — negative if a non-team word actually
// scores higher than anything on our team. assassinInTop3 is true when the
// assassin is among the 3 highest-scoring unrevealed words overall,
// regardless of margin (see operative.AllowBonus's doc comment for why that
// can't be folded into margin alone).
func (ai *AI) BonusGuessSignal(b *codenames.Board, ourAgent codenames.Agent, clueWord string) (margin float64, assassinInTop3 bool) {
	var unrevealed []codenames.Card
	for _, card := range b.Cards {
		if !card.Revealed {
			unrevealed = append(unrevealed, card)
		}
	}

	pairs := ai.rankBoardWords(unrevealed, clueWord)

	var topOurs, topOther float32
	haveOurs, haveOther := false, false
	for _, p := range pairs {
		if p.Agent == ourAgent {
			if !haveOurs {
				topOurs = p.Similarity
				haveOurs = true
			}
			continue
		}
		if !haveOther {
			topOther = p.Similarity
			haveOther = true
		}
	}
	if haveOurs && haveOther {
		margin = float64(topOurs - topOther)
	}

	top := min(3, len(pairs))
	for _, p := range pairs[:top] {
		if p.Agent == codenames.Assassin {
			assassinInTop3 = true
			break
		}
	}

	return margin, assassinInTop3
}

func (ai *AI) guessOrPass(b *codenames.Board, c *codenames.Clue, mustGuess bool) (string, string, error) {
	guessableTargets := codenames.Unused(b.Cards)
	pairs := ai.rankBoardWords(guessableTargets, c.Word)

	// This is a crutch for when the player enters a word that isn't in the model.
	if len(pairs) == 0 {
		return "", "no board words or clue word found in the embedding model", nil
	}

	if !mustGuess && pairs[0].Similarity < passThreshold {
		log.Printf("best guess %q for clue %q only scored %f, passing", pairs[0].Word, c.Word, pairs[0].Similarity)
		reasoning := fmt.Sprintf("best guess %q for clue %q only scored %.3f, below pass threshold %.2f — passing", pairs[0].Word, c.Word, pairs[0].Similarity, passThreshold)
		return codenames.PassGuess, reasoning, nil
	}

	return pairs[0].Word, formatGuessReasoning(pairs, c.Word), nil
}

// Similarity returns a value from 0 to 1, that is the similarity of the two
// input words.
func (ai *AI) similarity(a, b string) (float32, error) {
	s, err := ai.ConceptNetModel.Cos(exp(a), exp(b))
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

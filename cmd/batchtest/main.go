package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bcspragu/Codenames/codenames"
	"github.com/bcspragu/Codenames/game"
	"github.com/bcspragu/Codenames/llm"
)

var wordPool = strings.Fields(`
apple river mountain shadow ocean crown palace jack spider night krypton
superman cape guitar band stage rocket moon star planet doctor nurse
hospital knight castle dragon fire ice snow winter summer beach sand
desert cactus needle thread button shirt jacket coat hat glove sock
shoe boot ladder roof house door window glass mirror reflection lake
pond fish net rope anchor ship sail wind storm cloud rain umbrella
book page pen ink paper letter stamp mail box package gift ribbon bow
arrow bow target dart board game chess king queen pawn horse race
track field goal net ball bat glove helmet armor sword shield battle
war peace dove olive branch tree root leaf flower petal bee honey
comb hair brush paint canvas frame picture photo camera film movie
theater actor stage script pen sword pistol gun bullet trigger lock
key chain link fence wall brick stone rock boulder cliff cave bat
wing bird nest egg chick feather fly plane jet rocket engine wheel
car road street city town village farm barn cow milk cheese bread
butter jam toast egg pan stove oven kitchen table chair sofa couch
lamp light bulb switch wire circuit battery power plant factory
robot machine gear engine wheel bicycle chain pedal brake horn
`)

// uniqueWords is wordPool with duplicates removed. A board with the same
// codename on two cards is not a legal Codenames board: reveal() matches by
// name, so the second copy can never be guessed, and the prompt would list
// one word as belonging to two different teams at once.
var uniqueWords = dedupe(wordPool)

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, w := range in {
		if seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

func randomBoard(starter codenames.Team, seed int) *codenames.Board {
	r := rand.New(rand.NewSource(int64(seed)))
	words := make([]string, len(uniqueWords))
	copy(words, uniqueWords)
	r.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })
	words = words[:25]

	counts := map[codenames.Agent]int{
		codenames.RedAgent:  9,
		codenames.BlueAgent: 8,
		codenames.Bystander: 7,
		codenames.Assassin:  1,
	}
	if starter == codenames.BlueTeam {
		counts[codenames.BlueAgent], counts[codenames.RedAgent] = 9, 8
	}

	var agents []codenames.Agent
	for ag, c := range counts {
		for i := 0; i < c; i++ {
			agents = append(agents, ag)
		}
	}
	r.Shuffle(len(agents), func(i, j int) { agents[i], agents[j] = agents[j], agents[i] })

	cards := make([]codenames.Card, 25)
	seen := make(map[string]bool, 25)
	for i := range cards {
		if seen[words[i]] {
			log.Fatalf("duplicate codename %q on generated board (seed %d)", words[i], seed)
		}
		seen[words[i]] = true
		cards[i] = codenames.Card{Codename: words[i], Agent: agents[i]}
	}
	return &codenames.Board{Cards: cards}
}

// ---------------------------------------------------------------------------
// Instrumentation
//
// game.Play() drives the plain codenames.Spymaster/Operative interfaces, which
// expose only "a clue" and "a word". The interesting material for diagnosing a
// bad move — the spymaster's intended targets and its reason for each, and the
// operative's ranked candidates with confidences — lives in the richer llm
// methods. So rather than change the game package, we wrap each AI in a
// recorder that calls the rich method itself and satisfies the plain interface
// with the result.
// ---------------------------------------------------------------------------

// guessRecord is one guess (or pass) an operative made against a clue.
type guessRecord struct {
	Word      string // "" when the operative passed
	Pass      bool
	Actual    codenames.Agent // the card's true affiliation; UnknownAgent for a pass
	Correct   bool
	MustGuess bool
	Threshold float64
	Riskiest  string
	ParseErr  bool
	Cands     []llm.Candidate
}

// clueRecord is one clue and everything that happened because of it.
type clueRecord struct {
	Game     int
	Team     codenames.Team
	Word     string
	Count    int
	Targets  []string          // the words the spymaster meant
	Why      map[string]string // its stated reason per target
	Assassin string            // its assassin-check line
	Guesses  []guessRecord
}

// ourAgent is the agent this clue's team was trying to hit.
func (c *clueRecord) ourAgent() codenames.Agent {
	if c.Team == codenames.BlueTeam {
		return codenames.BlueAgent
	}
	return codenames.RedAgent
}

// reached reports whether the operative ever landed on word during this clue.
func (c *clueRecord) reached(word string) bool {
	for _, g := range c.Guesses {
		if strings.EqualFold(g.Word, word) {
			return true
		}
	}
	return false
}

type recorder struct {
	game   int
	agents map[string]codenames.Agent // true affiliation of every card, by lowercase codename
	clues  []*clueRecord
}

func newRecorder(game int, b *codenames.Board) *recorder {
	agents := make(map[string]codenames.Agent, len(b.Cards))
	for _, c := range b.Cards {
		agents[strings.ToLower(c.Codename)] = c.Agent
	}
	return &recorder{game: game, agents: agents}
}

// current returns the clue the given team is presently guessing against.
// Play() strictly alternates clue-then-guesses, so the team's most recent
// clue is always the live one.
func (r *recorder) current(team codenames.Team) *clueRecord {
	for i := len(r.clues) - 1; i >= 0; i-- {
		if r.clues[i].Team == team {
			return r.clues[i]
		}
	}
	return nil
}

// targetsFromReasoning pulls the target list and per-target WHY back out of
// the reasoning string parseClue builds ("Targets: a, b" followed by
// "  - a: reason" lines). Parsing the string rather than widening the AI's
// return type keeps this harness from forcing a signature change on the
// production code purely for measurement.
func targetsFromReasoning(reasoning string) ([]string, map[string]string, string) {
	var (
		targets  []string
		why      = map[string]string{}
		assassin string
	)
	for _, line := range strings.Split(reasoning, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "Assassin check:"):
			assassin = strings.TrimSpace(strings.TrimPrefix(t, "Assassin check:"))
		case strings.HasPrefix(t, "Targets:"):
			for _, w := range strings.Split(strings.TrimPrefix(t, "Targets:"), ",") {
				if w = strings.TrimSpace(w); w != "" {
					targets = append(targets, w)
				}
			}
		case strings.HasPrefix(t, "- "):
			word, reason, ok := strings.Cut(strings.TrimPrefix(t, "- "), ":")
			if ok {
				why[strings.ToLower(strings.TrimSpace(word))] = strings.TrimSpace(reason)
			}
		}
	}
	return targets, why, assassin
}

type trackedSpymaster struct {
	ai   *llm.AI
	rec  *recorder
	team codenames.Team
}

func (t *trackedSpymaster) GiveClue(b *codenames.Board, agent codenames.Agent) (*codenames.Clue, error) {
	clue, reasoning, err := t.ai.GiveClueWithReasoning(b, agent)
	if err != nil {
		return nil, err
	}
	targets, why, assassin := targetsFromReasoning(reasoning)
	t.rec.clues = append(t.rec.clues, &clueRecord{
		Game: t.rec.game, Team: t.team,
		Word: clue.Word, Count: clue.Count,
		Targets: targets, Why: why, Assassin: assassin,
	})
	return clue, nil
}

type trackedOperative struct {
	ai   *llm.AI
	rec  *recorder
	team codenames.Team
}

func (t *trackedOperative) Guess(b *codenames.Board, c *codenames.Clue) (string, error) {
	cur := t.rec.current(t.team)
	if cur == nil {
		return "", fmt.Errorf("operative asked to guess with no recorded clue for %s", t.team)
	}

	// Only correct guesses keep a turn alive, so the number already made is
	// exactly the length of the guess list; the first is the mandated one.
	var revealed []string
	for _, g := range cur.Guesses {
		revealed = append(revealed, g.Word)
	}
	mustGuess := len(cur.Guesses) == 0

	res, err := t.ai.GuessWithCandidates(b, c, string(t.team), mustGuess, len(cur.Guesses), revealed)
	if err != nil {
		return "", err
	}

	gr := guessRecord{
		MustGuess: res.MustGuess, Threshold: res.ThresholdApplied,
		Riskiest: res.RiskiestBoardWord, ParseErr: res.ParseError, Cands: res.Candidates,
	}
	// A timed-out or unparseable guess comes back with an empty Guess, which
	// game.Play() then treats as a deliberate pass. That makes a broken run
	// look like a cautious one, so it has to be counted separately or the
	// search would happily "prefer" a config that is merely failing.
	if res.ParseError {
		gr.Pass = true
	}
	switch res.Guess {
	case codenames.PassGuess, "":
		gr.Pass = true
	default:
		gr.Word = res.Guess
		gr.Actual = t.rec.agents[strings.ToLower(res.Guess)]
		gr.Correct = gr.Actual == cur.ourAgent()
	}
	cur.Guesses = append(cur.Guesses, gr)
	return res.Guess, nil
}

func agentName(a codenames.Agent) string {
	switch a {
	case codenames.RedAgent:
		return "RED"
	case codenames.BlueAgent:
		return "BLUE"
	case codenames.Bystander:
		return "BYSTANDER"
	case codenames.Assassin:
		return "ASSASSIN"
	}
	return "UNKNOWN"
}

// envFloat reads a tunable from the environment, falling back to def. Every
// knob the search loop turns is read this way so an iteration is a config
// change rather than a code edit and rebuild.
func envFloat(name string, def float64) float64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Fatalf("%s=%q is not a number: %v", name, v, err)
	}
	return f
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("%s=%q is not an integer: %v", name, v, err)
	}
	return n
}

func envBool(name string, def bool) bool {
	switch os.Getenv(name) {
	case "":
		return def
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func main() {
	endpoint := "http://localhost:11434"
	model := "qwen3:30b-a3b"
	if m := os.Getenv("MODEL"); m != "" {
		model = m
	}

	numGames := envInt("GAMES", 5)
	// Board seeds are fixed and shared across every run, so two configs are
	// always compared on identical boards. Without this, a config's score
	// would move with board luck and the search would chase noise.
	seedBase := envInt("SEED_BASE", 1000)

	// Thinking is per role: the spymaster's line-based schema needs the
	// scratch space, while the operative's JSON reply is constrained by
	// Ollama's structured-output mode and may not.
	smThink := envBool("SM_THINK", true)
	opThink := envBool("OP_THINK", true)
	verbose := envBool("VERBOSE", false)

	cfg := llm.GuessDecisionConfig{
		MandatedThreshold:   envFloat("MANDATED_THRESHOLD", llm.DefaultGuessDecisionConfig.MandatedThreshold),
		BonusThreshold:      envFloat("BONUS_THRESHOLD", llm.DefaultGuessDecisionConfig.BonusThreshold),
		RiskiestWordPenalty: envFloat("RISKIEST_PENALTY", llm.DefaultGuessDecisionConfig.RiskiestWordPenalty),
		LinkTypeCaps: map[string]float64{
			"direct":    envFloat("CAP_DIRECT", llm.DefaultGuessDecisionConfig.LinkTypeCaps["direct"]),
			"category":  envFloat("CAP_CATEGORY", llm.DefaultGuessDecisionConfig.LinkTypeCaps["category"]),
			"idiom":     envFloat("CAP_IDIOM", llm.DefaultGuessDecisionConfig.LinkTypeCaps["idiom"]),
			"multi_hop": envFloat("CAP_MULTIHOP", llm.DefaultGuessDecisionConfig.LinkTypeCaps["multi_hop"]),
		},
		UnknownLinkTypeCap: envFloat("CAP_UNKNOWN", llm.DefaultGuessDecisionConfig.UnknownLinkTypeCap),
	}
	temp := envFloat("TEMPERATURE", llm.DefaultTemperature)

	outDir := os.Getenv("OUT_DIR")
	if outDir == "" {
		outDir = "/tmp/claude-1718309039/-home-ec036824-ai-codenames/7df494cd-60dc-4878-ab17-18410dbbe918/scratchpad/batch"
	}

	f, err := os.Create(outDir + "/run.log")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)

	fmt.Printf("config: model=%s games=%d seeds=%d.. max_tokens=%d sm_think=%v op_think=%v mandated=%.2f bonus=%.2f caps=[d %.2f c %.2f i %.2f m %.2f] temp=%.2f\n",
		model, numGames, seedBase, envInt("MAX_TOKENS", 0), smThink, opThink, cfg.MandatedThreshold, cfg.BonusThreshold,
		cfg.LinkTypeCaps["direct"], cfg.LinkTypeCaps["category"], cfg.LinkTypeCaps["idiom"], cfg.LinkTypeCaps["multi_hop"], temp)

	// A reasoning model that runs out of num_predict mid-<think> produces
	// nothing usable, and the whole generation is wasted before the retry
	// doubles the budget. Starting high trades a larger cap for far fewer
	// wasted generations, which dominates wall-clock time.
	maxTokens := envInt("MAX_TOKENS", 0)
	// Concurrency raises per-request latency even when it raises total
	// throughput: N games queue against one GPU, so every individual call
	// waits longer. The default 3-minute timeout is sized for sequential
	// play and will fire spuriously under parallelism.
	timeout := time.Duration(envInt("TIMEOUT_SEC", 0)) * time.Second
	mk := func(think bool) *llm.AI {
		return llm.New(endpoint, model, timeout, maxTokens,
			llm.WithThink(think), llm.WithVerboseLogs(verbose),
			llm.WithGuessDecisionConfig(cfg), llm.WithTemperature(temp))
	}

	// Games are independent, so they can run concurrently against the same
	// Ollama server. Whether that actually helps depends on the server's
	// OLLAMA_NUM_PARALLEL and available VRAM — if it serializes, this costs
	// nothing but gains nothing.
	parallel := envInt("PARALLEL", 1)
	if parallel < 1 {
		parallel = 1
	}

	var (
		mu       sync.Mutex
		all      []*clueRecord
		wg       sync.WaitGroup
		sem      = make(chan struct{}, parallel)
		runStart = time.Now()
	)
	for i := 0; i < numGames; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			starter := codenames.RedTeam
			if i%2 == 1 {
				starter = codenames.BlueTeam
			}
			b := randomBoard(starter, seedBase+i)
			rec := newRecorder(i, b)

			g, err := game.New(b, starter, &game.Config{
				RedSpymaster:  &trackedSpymaster{mk(smThink), rec, codenames.RedTeam},
				RedOperative:  &trackedOperative{mk(opThink), rec, codenames.RedTeam},
				BlueSpymaster: &trackedSpymaster{mk(smThink), rec, codenames.BlueTeam},
				BlueOperative: &trackedOperative{mk(opThink), rec, codenames.BlueTeam},
			})
			if err != nil {
				fmt.Printf("game %d: setup error: %v\n", i, err)
				return
			}

			start := time.Now()
			outcome, err := g.Play()

			mu.Lock()
			all = append(all, rec.clues...)
			mu.Unlock()

			if err != nil {
				fmt.Printf("game %d: play error after %s: %v\n", i, time.Since(start).Round(time.Second), err)
				return
			}
			fmt.Printf("game %d: winner=%-4s clues=%2d elapsed=%s\n",
				i, outcome.Winner, len(rec.clues), time.Since(start).Round(time.Second))
		}(i)
	}
	wg.Wait()
	fmt.Printf("total elapsed: %s (parallel=%d)\n", time.Since(runStart).Round(time.Second), parallel)

	report(all, numGames, outDir)
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// flag kinds, most damaging first.
const (
	flagAssassin  = "ASSASSIN HIT"
	flagOpponent  = "GAVE OPPONENT A WORD"
	flagBystander = "BYSTANDER"
	flagPartial   = "PARTIAL CLUE"  // some targets landed, then a miss
	flagUnreached = "TARGET MISSED" // turn ended with a target never guessed
	flagParse     = "PARSE FAILURE"
)

type finding struct {
	Kind  string
	Clue  *clueRecord
	Guess *guessRecord // nil for a clue-level finding
	Note  string
}

// classify walks a clue and everything that happened under it, emitting a
// finding per problem. A single clue can produce several: a wrong guess is a
// guess-level fault, while targets the operative never reached is a
// clue-level one, and they have different causes.
func classify(c *clueRecord) []finding {
	var out []finding

	// A clue whose first target(s) landed and then produced a miss is the
	// specific failure worth separating out: the link was real for one word
	// but not the next, which is a spymaster fault (over-reaching on the
	// second word), not an operative fault.
	landedBefore := 0
	for i := range c.Guesses {
		g := &c.Guesses[i]
		if g.ParseErr {
			out = append(out, finding{flagParse, c, g, "every retry failed to parse; a random legal card was played"})
			continue
		}
		if g.Pass {
			continue
		}
		if g.Correct {
			landedBefore++
			continue
		}

		kind := flagBystander
		switch g.Actual {
		case codenames.Assassin:
			kind = flagAssassin
		case codenames.RedAgent, codenames.BlueAgent:
			kind = flagOpponent
		}
		note := fmt.Sprintf("guess %d of a %d-clue", i+1, c.Count)
		if landedBefore > 0 {
			note += fmt.Sprintf("; the first %d target(s) landed, so the clue's link held for those and broke on this one", landedBefore)
		}
		out = append(out, finding{kind, c, g, note})

		if landedBefore > 0 {
			out = append(out, finding{flagPartial, c, g, fmt.Sprintf(
				"clue %q(%d): %d of %d intended targets found, then the operative went to %q (%s)",
				c.Word, c.Count, landedBefore, c.Count, g.Word, agentName(g.Actual))})
		}
	}

	// Targets the spymaster meant but the operative never reached.
	var missed []string
	for _, t := range c.Targets {
		if !c.reached(t) {
			missed = append(missed, t)
		}
	}
	if len(missed) > 0 && len(c.Targets) > 0 {
		out = append(out, finding{flagUnreached, c, nil, fmt.Sprintf(
			"never guessed: %s (of intended %s)", strings.Join(missed, ", "), strings.Join(c.Targets, ", "))})
	}
	return out
}

func report(all []*clueRecord, games int, outDir string) {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	var findings []finding
	totalGuesses, correct := 0, 0
	byKind := map[string]int{}
	for _, c := range all {
		for _, g := range c.Guesses {
			if g.Pass {
				continue
			}
			totalGuesses++
			if g.Correct {
				correct++
			}
		}
		for _, f := range classify(c) {
			findings = append(findings, f)
			byKind[f.Kind]++
		}
	}

	p("# Batch report — %d games, %d clues, %d guesses\n\n", games, len(all), totalGuesses)
	pct := 0.0
	if totalGuesses > 0 {
		pct = 100 * float64(correct) / float64(totalGuesses)
	}
	p("Guess accuracy: %d/%d (%.0f%%)\n\n", correct, totalGuesses, pct)

	p("## Flags\n\n")
	for _, k := range []string{flagAssassin, flagOpponent, flagBystander, flagPartial, flagUnreached, flagParse} {
		p("  %-22s %d\n", k, byKind[k])
	}

	// Clue-size breakdown: if over-reaching on the 2nd word is the pattern,
	// it shows up as a worse landing rate for 3s than for 2s.
	p("\n## Landing rate by clue size\n\n")
	type bucket struct{ clues, targets, found int }
	sizes := map[int]*bucket{}
	for _, c := range all {
		bk := sizes[c.Count]
		if bk == nil {
			bk = &bucket{}
			sizes[c.Count] = bk
		}
		bk.clues++
		for _, t := range c.Targets {
			bk.targets++
			if c.reached(t) {
				bk.found++
			}
		}
	}
	for n := 1; n <= 5; n++ {
		if bk := sizes[n]; bk != nil && bk.targets > 0 {
			p("  count=%d: %2d clues, %2d/%2d intended targets found (%.0f%%)\n",
				n, bk.clues, bk.found, bk.targets, 100*float64(bk.found)/float64(bk.targets))
		}
	}

	p("\n## Flagged moves\n")
	for _, f := range findings {
		if f.Kind == flagPartial {
			continue // reported inline on the guess itself
		}
		c := f.Clue
		p("\n---\n\n### %s — game %d, %s\n\n", f.Kind, c.Game, c.Team)
		p("**Clue:** `%s (%d)`  →  intended targets: %s\n\n", c.Word, c.Count, strings.Join(c.Targets, ", "))
		if len(c.Why) > 0 {
			p("Spymaster's reasoning per target:\n")
			for _, t := range c.Targets {
				if w := c.Why[strings.ToLower(t)]; w != "" {
					p("  - **%s** — %s\n", t, w)
				} else {
					p("  - **%s** — (no reason given)\n", t)
				}
			}
			p("\n")
		}
		if c.Assassin != "" {
			p("Spymaster's assassin check: %s\n\n", c.Assassin)
		}
		p("What happened: %s\n\n", f.Note)

		if f.Guess != nil {
			g := f.Guess
			p("**Operative guessed `%s` — actually %s.**", strings.ToUpper(g.Word), agentName(g.Actual))
			if g.MustGuess {
				p(" (mandated first guess, no threshold)")
			} else {
				p(" (threshold %.2f)", g.Threshold)
			}
			p("\n\nIts ranked candidates:\n")
			for i, cd := range g.Cands {
				mark := " "
				if i == 0 {
					mark = "▸"
				}
				p("  %s %-12s conf %.2f  %-10s — %s\n", mark, cd.Word, cd.Confidence, cd.LinkType, cd.Reasoning)
			}
			if g.Riskiest != "" {
				p("\n  Operative named `%s` as the riskiest board word.\n", g.Riskiest)
			}
		}
	}

	// The search loop reads this, not the prose report. Landing rate is the
	// primary signal: of every word a spymaster actually meant, how many did
	// its operative reach? It measures the clue-giver/guesser coordination
	// directly and, unlike win rate, isn't diluted by the opponent's luck.
	var targets, found, secondPlus, secondPlusOK, parseErrors int
	assassins, opponents, bystanders := 0, 0, 0
	for _, c := range all {
		for _, g := range c.Guesses {
			if g.ParseErr {
				parseErrors++
			}
		}
		for _, t := range c.Targets {
			targets++
			if c.reached(t) {
				found++
			}
		}
		n := 0
		for _, g := range c.Guesses {
			if g.Pass {
				continue
			}
			if n > 0 {
				secondPlus++
				if g.Correct {
					secondPlusOK++
				}
			}
			n++
			if g.Correct {
				continue
			}
			switch g.Actual {
			case codenames.Assassin:
				assassins++
			case codenames.RedAgent, codenames.BlueAgent:
				opponents++
			default:
				bystanders++
			}
		}
	}
	ratio := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) / float64(b)
	}
	metrics := map[string]any{
		"games":            games,
		"clues":            len(all),
		"guesses":          totalGuesses,
		"correct":          correct,
		"guess_accuracy":   ratio(correct, totalGuesses),
		"landing_rate":     ratio(found, targets),
		"targets_intended": targets,
		"targets_found":    found,
		"second_plus":      secondPlus,
		"second_plus_ok":   secondPlusOK,
		"second_plus_rate": ratio(secondPlusOK, secondPlus),
		"assassin_hits":    assassins,
		"opponent_gifts":   opponents,
		"bystander_hits":   bystanders,
		// Non-zero means the run is degraded and its other numbers are not
		// comparable against a clean run.
		"parse_errors": parseErrors,
	}
	mj, _ := json.MarshalIndent(metrics, "", "  ")
	if err := os.WriteFile(outDir+"/metrics.json", mj, 0o644); err != nil {
		log.Printf("failed to write metrics: %v", err)
	}
	p("\n## Metrics\n\n```json\n%s\n```\n", mj)

	out := b.String()
	fmt.Println(out)
	path := outDir + "/report.md"
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		log.Printf("failed to write report: %v", err)
	}
	fmt.Println("Report:", path)
	fmt.Println("Full AI log:", outDir+"/run.log")
}

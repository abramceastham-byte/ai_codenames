// Binary codenames-local allows playing a game on the command-line, including
// using AI players.
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/bcspragu/Codenames/boardgen"
	"github.com/bcspragu/Codenames/codenames"
	"github.com/bcspragu/Codenames/game"
	"github.com/bcspragu/Codenames/io"
	"github.com/bcspragu/Codenames/llm"
	"github.com/bcspragu/Codenames/w2v"
)

var (
	teamMap = map[string]codenames.Team{
		"red":  codenames.RedTeam,
		"blue": codenames.BlueTeam,
	}
	agentMap = map[string]codenames.Agent{
		"red":       codenames.RedAgent,
		"blue":      codenames.BlueAgent,
		"bystander": codenames.Bystander,
		"assassin":  codenames.Assassin,
	}
)

func main() {
	var (
		gloveFile      = flag.String("glove_file", "glove.bin", "A binary-formatted GloVe word2vec model file.")
		conceptNetFile = flag.String("concept_net_file", "conceptnet.bin", "A binary-formatted ConceptNet word2vec model file.")
		commonWordlist = flag.String("common_wordlist", "common_words.txt", "Path to a common words file for AI clue generation.")
		wordList       = flag.String("words", "", "Comma-separated list of words and the agent they're assigned to. Ex dog:red,wallet:blue,bowl:assassin,glass:blue,hood:bystander")
		starter        = flag.String("starter", "red", "Which color team starts the game")
		team           = flag.String("team", "red", "Team to be")
		useAI          = flag.Bool("use_ai", false, "Whether or not the starting team should be an AI.")
		aiBackend      = flag.String("ai_backend", "w2v", "AI backend to use: 'w2v' or 'llm'")
		ollamaEndpoint = flag.String("ollama_endpoint", "http://localhost:11434", "Ollama API endpoint")
		ollamaModel    = flag.String("ollama_model", "llama3", "Ollama model name")
	)
	flag.Parse()

	if err := validColor(*starter); err != nil {
		log.Fatal(err)
	}

	if err := validColor(*team); err != nil {
		log.Fatal(err)
	}

	var b *codenames.Board
	if *wordList == "" {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		b = boardgen.New(teamMap[*starter], r)
	} else {
		words := strings.Split(*wordList, ",")
		if len(words) != codenames.Size {
			log.Fatalf("Expected %d words, got %d words", codenames.Size, len(words))
		}
		cards := make([]codenames.Card, len(words))
		for i, w := range words {
			c, err := parseCard(w)
			if err != nil {
				log.Fatalf("Failed on card #%d: %q: %v", i, w, err)
			}
			cards[i] = c
		}
		b = &codenames.Board{Cards: cards}
	}

	var (
		sm codenames.Spymaster
		op codenames.Operative
	)
	if *useAI {
		switch *aiBackend {
		case "w2v":
			ai, err := w2v.New(*gloveFile, *conceptNetFile, *commonWordlist)
			if err != nil {
				log.Fatalf("Failed to initialize word2vec model: %v", err)
			}
			sm, op = ai, ai
		case "llm":
			ai := llm.New(*ollamaEndpoint, *ollamaModel)
			sm, op = ai, ai
		default:
			log.Fatalf("Unknown AI backend %q, must be 'w2v' or 'llm'", *aiBackend)
		}
	}

	var (
		rsm codenames.Spymaster = &io.Spymaster{In: os.Stdin, Out: os.Stdout}
		bsm codenames.Spymaster = &io.Spymaster{In: os.Stdin, Out: os.Stdout}

		rop codenames.Operative = &io.Operative{In: os.Stdin, Out: os.Stdout, Team: codenames.RedTeam}
		bop codenames.Operative = &io.Operative{In: os.Stdin, Out: os.Stdout, Team: codenames.BlueTeam}
	)

	if *useAI {
		switch teamMap[*team] {
		case codenames.RedTeam:
			rsm, rop = sm, op
		case codenames.BlueTeam:
			bsm, bop = sm, op
		}
	}

	g, err := game.New(b, teamMap[*starter], &game.Config{
		RedSpymaster:  rsm,
		BlueSpymaster: bsm,
		RedOperative:  rop,
		BlueOperative: bop,
	})
	if err != nil {
		log.Fatalf("Failed to instantiate game: %v", err)
	}

	fmt.Println(g.Play())
}

func validColor(c string) error {
	switch c {
	case "red":
		return nil
	case "blue":
		return nil
	default:
		return fmt.Errorf("invalid team color %q, 'red' and 'blue' are the only valid team colors", c)
	}
}

func parseCard(in string) (codenames.Card, error) {
	ps := strings.Split(in, ":")
	if len(ps) != 2 {
		return codenames.Card{}, fmt.Errorf("malformed card string %q", in)
	}

	ag, ok := agentMap[strings.ToLower(ps[1])]
	if !ok {
		return codenames.Card{}, fmt.Errorf("invalid agent type %q", ps[1])
	}

	return codenames.Card{Codename: ps[0], Agent: ag}, nil
}

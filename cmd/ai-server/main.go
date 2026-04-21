// Binary ai-server provides an AI implementation of a Codenames client.
// It supports Word2Vec and LLM (Ollama) backends, and can host multiple
// backends concurrently so callers can pick which one joins a given game.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/bcspragu/Codenames/cryptorand"
	"github.com/bcspragu/Codenames/llm"
	"github.com/bcspragu/Codenames/w2v"

	ff "github.com/peterbourgon/ff/v4"
)

func main() {
	if err := run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("no args given")
	}

	fSet := flag.NewFlagSet(args[0], flag.ContinueOnError)
	var (
		enabledBackends     = fSet.String("enabled_backends", "w2v,llm", "Comma-separated list of backends to enable. Each named backend must have its required config provided.")
		defaultBackend      = fSet.String("default_backend", "", "Which backend to use when a caller doesn't specify. Defaults to the first enabled backend.")
		gloveModelPath      = fSet.String("glove_model_path", "", "Path to binary Word2Vec GloVe model data (required if w2v is enabled)")
		conceptNetModelPath = fSet.String("concept_net_model_path", "", "Path to binary Word2Vec ConceptNet model data (required if w2v is enabled)")
		authSecret          = fSet.String("auth_secret", "", "Secret string that callers must provide")
		commonWordlist      = fSet.String("common_wordlist", "", "Path to word list of most common words, used for making guesses")
		webServerEndpoint   = fSet.String("web_server_endpoint", "", "The address to connect to the Codenames game web server")
		ollamaEndpoint      = fSet.String("ollama_endpoint", "http://localhost:11434", "Ollama API endpoint")
		ollamaModel         = fSet.String("ollama_model", "llama3", "Ollama model name")
	)
	if err := ff.Parse(fSet, args[1:], ff.WithEnvVars()); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if *authSecret == "" {
		return errors.New("--auth_secret must be provided")
	}

	if *webServerEndpoint == "" {
		return errors.New("--web_server_endpoint must be provided")
	}

	ais := make(map[string]AI)
	for _, name := range splitCSV(*enabledBackends) {
		switch name {
		case "w2v":
			if *gloveModelPath == "" || *conceptNetModelPath == "" {
				return errors.New("w2v backend requires --glove_model_path and --concept_net_model_path")
			}
			log.Printf("Loading Word2Vec backend from %s and %s", *gloveModelPath, *conceptNetModelPath)
			w2vAI, err := w2v.New(*gloveModelPath, *conceptNetModelPath, *commonWordlist)
			if err != nil {
				return fmt.Errorf("failed to load w2v backend: %w", err)
			}
			ais["w2v"] = w2vAI
		case "llm":
			log.Printf("Loading LLM backend via Ollama at %s with model %s", *ollamaEndpoint, *ollamaModel)
			ais["llm"] = llm.New(*ollamaEndpoint, *ollamaModel)
		default:
			return fmt.Errorf("unknown backend %q in --enabled_backends", name)
		}
	}

	if len(ais) == 0 {
		return errors.New("no AI backends enabled; set --enabled_backends")
	}

	def := *defaultBackend
	if def == "" {
		def = firstKey(ais)
	}
	if _, ok := ais[def]; !ok {
		return fmt.Errorf("--default_backend %q is not among enabled backends", def)
	}
	log.Printf("Default backend: %s (available: %s)", def, strings.Join(sortedKeys(ais), ", "))

	r := rand.New(cryptorand.NewSource())

	srv := newServer(ais, def, *authSecret, *webServerEndpoint, r)

	if err := http.ListenAndServe(":8081", srv); err != nil {
		return fmt.Errorf("error from server: %w", err)
	}

	return nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sortedKeys(m map[string]AI) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func firstKey(m map[string]AI) string {
	return sortedKeys(m)[0]
}

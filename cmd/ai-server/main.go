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
	"path/filepath"
	"sort"
	"strconv"
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
		ollamaModel         = fSet.String("ollama_model", "qwq:32b", "Ollama model name")
		ollamaTimeout       = fSet.Duration("ollama_timeout", llm.DefaultTimeout, "Max time to wait for an Ollama call (and total budget across guess retries) before falling back to a random legal move")
		ollamaMaxTokens     = fSet.Int("ollama_max_tokens", llm.DefaultMaxTokens, "Max tokens Ollama may generate per reply (thinking + answer), passed as num_predict; the real lever on response latency for a reasoning model")
		ollamaTemperature   = fSet.Float64("ollama_temperature", llm.DefaultTemperature, "Ollama sampling temperature (higher = more creative/random word choices). Sent on every call so it's a fixed, known number for research comparisons instead of an implicit Ollama default")
		ollamaSeed          = fSet.Int("ollama_seed", -1, "Ollama sampling seed; -1 (default) leaves sampling random so games vary. Set a specific value to make replies to identical prompts reproducible, e.g. for research runs")
		ollamaThink         = fSet.String("ollama_think", "", "Set Ollama's top-level \"think\" field to force reasoning on/off for models that support toggling it (e.g. qwen3): \"true\" or \"false\". Empty (default) sends nothing, leaving qwq and other pure reasoning models unaffected")
		reasoningLogPath    = fSet.String("reasoning_log_path", "logs/ai_reasoning.jsonl", "Path to JSONL file where AI reasoning for clues/guesses is logged")
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
			llmOpts := []llm.Option{llm.WithTemperature(*ollamaTemperature)}
			if *ollamaSeed != -1 {
				llmOpts = append(llmOpts, llm.WithSeed(*ollamaSeed))
			}
			thinkDesc := "unset"
			if *ollamaThink != "" {
				think, err := strconv.ParseBool(*ollamaThink)
				if err != nil {
					return fmt.Errorf("--ollama_think %q must be \"true\" or \"false\": %w", *ollamaThink, err)
				}
				llmOpts = append(llmOpts, llm.WithThink(think))
				thinkDesc = strconv.FormatBool(think)
			}
			log.Printf("Loading LLM backend via Ollama at %s with model %s (timeout %s, max tokens %d, temperature %v, seed %d, think %s)", *ollamaEndpoint, *ollamaModel, *ollamaTimeout, *ollamaMaxTokens, *ollamaTemperature, *ollamaSeed, thinkDesc)
			ais["llm"] = llm.New(*ollamaEndpoint, *ollamaModel, *ollamaTimeout, *ollamaMaxTokens, llmOpts...)
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

	if err := os.MkdirAll(filepath.Dir(*reasoningLogPath), 0755); err != nil {
		return fmt.Errorf("failed to create reasoning log dir: %w", err)
	}
	reasoningLog, err := os.OpenFile(*reasoningLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open reasoning log: %w", err)
	}
	log.Printf("Logging AI reasoning to %s", *reasoningLogPath)

	srv := newServer(ais, def, *authSecret, *webServerEndpoint, r, reasoningLog, *reasoningLogPath)

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

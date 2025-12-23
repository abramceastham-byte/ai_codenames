// Binary ai-server provides an AI implementation of a Codenames client.
// It supports Word2Vec-based backends.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"

	"github.com/bcspragu/Codenames/cryptorand"
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
		gloveModelPath      = fSet.String("glove_model_path", "", "Path to binary Word2Vec GloVe model data")
		conceptNetModelPath = fSet.String("concept_net_model_path", "", "Path to binary Word2Vec ConceptNet model data")
		authSecret          = fSet.String("auth_secret", "", "Secret string that callers must provide")
		commonWordlist      = fSet.String("common_wordlist", "", "Path to word list of most common words, used for making guesses")
		webServerEndpoint   = fSet.String("web_server_endpoint", "", "The address to connect to the Codenames game web server")
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

	if *gloveModelPath == "" || *conceptNetModelPath == "" {
		return errors.New("--{glove,concept_net}_model_path must be provided")
	}

	log.Printf("Using Word2Vec models from %s and %s", *gloveModelPath, *conceptNetModelPath)
	ai, err := w2v.New(*gloveModelPath, *conceptNetModelPath, *commonWordlist)
	if err != nil {
		return fmt.Errorf("failed to load AI: %w", err)
	}

	r := rand.New(cryptorand.NewSource())

	srv := newServer(ai, *authSecret, *webServerEndpoint, r)

	if err := http.ListenAndServe(":8081", srv); err != nil {
		return fmt.Errorf("error from server: %w", err)
	}

	return nil
}

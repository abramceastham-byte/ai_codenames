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
		operativeModelPath = fSet.String("operative_model_path", "", "Path to binary Word2Vec operative model data")
		spymasterModelPath = fSet.String("spymaster_model_path", "", "Path to binary Word2Vec spymaster model data")
		authSecret         = fSet.String("auth_secret", "", "Secret string that callers must provide")
		webServerEndpoint  = fSet.String("web_server_endpoint", "", "The address to connect to the Codenames game web server")
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

	if *operativeModelPath == "" || *spymasterModelPath == "" {
		return errors.New("--{operative,spymaster}_model_path must be provided")
	}

	log.Printf("Using Word2Vec models from %s and %s", *operativeModelPath, *spymasterModelPath)
	ai, err := w2v.New(*operativeModelPath, *spymasterModelPath)
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

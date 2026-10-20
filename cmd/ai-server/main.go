// Binary ai-server hosts bot players for the Codenames web server. Bots are
// registered by name in the backends map below, and callers pick which one
// joins a given game.
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

	ff "github.com/peterbourgon/ff/v4"
)

// backends maps a bot name to a constructor for it. Add a tournament bot by
// registering a func returning a value that implements AI (see server.go).
var backends = map[string]func() (AI, error){}

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
		enabledBackends   = fSet.String("enabled_backends", "", "Comma-separated list of backends to enable. Defaults to all registered backends.")
		defaultBackend    = fSet.String("default_backend", "", "Which backend to use when a caller doesn't specify. Defaults to the first enabled backend.")
		authSecret        = fSet.String("auth_secret", "", "Secret string that callers must provide")
		webServerEndpoint = fSet.String("web_server_endpoint", "", "The address to connect to the Codenames game web server")
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

	enabled := splitCSV(*enabledBackends)
	if len(enabled) == 0 {
		enabled = registeredNames()
	}

	ais := make(map[string]AI)
	for _, name := range enabled {
		newAI, ok := backends[name]
		if !ok {
			return fmt.Errorf("unknown backend %q in --enabled_backends (registered: %s)", name, strings.Join(registeredNames(), ", "))
		}
		ai, err := newAI()
		if err != nil {
			return fmt.Errorf("failed to load %s backend: %w", name, err)
		}
		ais[name] = ai
	}

	if len(ais) == 0 {
		return errors.New("no AI backends registered; add one to the backends map in cmd/ai-server/main.go")
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

func registeredNames() []string {
	out := make([]string, 0, len(backends))
	for k := range backends {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

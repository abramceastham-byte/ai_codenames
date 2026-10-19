// Binary codenames-server provides an HTTP-based API server that can manage
// games in a SQLite database.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bcspragu/Codenames/aiclient"
	"github.com/bcspragu/Codenames/cryptorand"
	"github.com/bcspragu/Codenames/sqldb"
	"github.com/bcspragu/Codenames/web"
	"github.com/gorilla/securecookie"
	"github.com/rs/cors"

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
		addr   = fSet.String("addr", ":8080", "HTTP service address")
		dbPath = fSet.String("db_path", "codenames.db", "Path to the SQLite DB file")

		hashKeyPath  = fSet.String("hash_key_path", "hashKey", "Path to the hash key for secure cookies")
		blockKeyPath = fSet.String("block_key_path", "blockKey", "Path to the block key for secure cookies")

		// AI server-related flags
		authSecret       = fSet.String("auth_secret", "", "Secret string that acts as a 'password' for communicating with the AI server")
		aiServerEndpoint = fSet.String("ai_server_endpoint", "", "The address to connect to the Codenames AI server")
		logDir           = fSet.String("log_dir", "logs", "Directory to save game logs (CSV) after each game")
		allowedOrigin    = fSet.String("allowed_origin", "http://localhost:5173", "Browser origin allowed for CORS and WebSockets")
		secureCookies    = fSet.Bool("secure_cookies", false, "Only send authentication cookies over HTTPS")
		staticDir        = fSet.String("static_dir", "", "Directory containing the built frontend; empty disables static serving")
	)
	if err := ff.Parse(fSet, args[1:], ff.WithEnvVars()); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	if _, err := url.ParseRequestURI(*allowedOrigin); err != nil {
		return fmt.Errorf("invalid --allowed_origin: %w", err)
	}

	r := rand.New(cryptorand.NewSource())
	db, err := sqldb.New(*dbPath, r)
	if err != nil {
		return fmt.Errorf("failed to initialize datastore: %w", err)
	}

	sc, err := loadKeys(*hashKeyPath, *blockKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load cookie keys: %w", err)
	}

	ai := aiclient.New(*authSecret, *aiServerEndpoint)

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		if err := db.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
		os.Exit(1)
	}()

	log.Printf("Server is running on %q", *addr)

	webSrv := web.New(db, r, sc, ai, *logDir, web.Options{
		AllowedOrigins: []string{*allowedOrigin},
		SecureCookies:  *secureCookies,
	})
	app := http.Handler(webSrv)
	if *staticDir != "" {
		app, err = withStaticFrontend(app, *staticDir)
		if err != nil {
			return err
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := db.Ping(); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/", app)
	corsCfg := cors.New(cors.Options{
		AllowedOrigins:   []string{*allowedOrigin},
		AllowCredentials: true,
		AllowedHeaders:   []string{"Content-Type"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch},
	})

	server := &http.Server{Addr: *addr, Handler: corsCfg.Handler(mux), ReadHeaderTimeout: 10 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("ListenAndServe: %w", err)
	}

	return nil
}

func withStaticFrontend(api http.Handler, dir string) (http.Handler, error) {
	index := filepath.Join(dir, "200.html")
	if _, err := os.Stat(index); err != nil {
		return nil, fmt.Errorf("frontend fallback %q is unavailable: %w", index, err)
	}
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, index)
	}), nil
}

func loadKeys(hashKeyFileName, blockKeyFileName string) (*securecookie.SecureCookie, error) {
	hashKey, err := loadOrGenKey(hashKeyFileName)
	if err != nil {
		return nil, err
	}

	blockKey, err := loadOrGenKey(blockKeyFileName)
	if err != nil {
		return nil, err
	}

	return securecookie.New(hashKey, blockKey), nil
}

func loadOrGenKey(name string) ([]byte, error) {
	f, err := os.ReadFile(name)
	if err == nil {
		return f, nil
	}

	dat := securecookie.GenerateRandomKey(32)
	if dat == nil {
		return nil, errors.New("failed to generate key")
	}

	err = os.WriteFile(name, dat, 0600)
	if err != nil {
		return nil, errors.New("error writing file")
	}
	return dat, nil
}

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
	"os"
	"os/signal"
	"syscall"

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
	)
	if err := ff.Parse(fSet, args[1:], ff.WithEnvVars()); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	flag.Parse()

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

	webSrv := web.New(db, r, sc, ai)
	corsCfg := cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowCredentials: true,
	})

	if err := http.ListenAndServe(*addr, corsCfg.Handler(webSrv)); err != nil {
		return fmt.Errorf("ListenAndServe: %w", err)
	}

	return nil
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

	err = os.WriteFile(name, dat, 0777)
	if err != nil {
		return nil, errors.New("error writing file")
	}
	return dat, nil
}

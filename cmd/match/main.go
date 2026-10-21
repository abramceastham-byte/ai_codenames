// Binary match plays one game between two teams of JSON bots (see
// BOT_GUIDE.md), writes a JSONL move log, and appends the outcome to a
// results file.
//
// Example:
//
//	go run ./cmd/match \
//	  --red_name alpha \
//	  --red_spymaster "python3 examples/python/spymaster.py" \
//	  --red_operative "python3 examples/python/operative.py" \
//	  --blue_name beta \
//	  --blue_spymaster "python3 examples/python/spymaster.py" \
//	  --blue_operative "python3 examples/python/operative.py"
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bcspragu/Codenames/boardgen"
	"github.com/bcspragu/Codenames/botproto"
	"github.com/bcspragu/Codenames/codenames"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		redName       = flag.String("red_name", "red", "Name of the red team")
		redSpymaster  = flag.String("red_spymaster", "", "Command for the red spymaster bot (split on spaces, no shell)")
		redOperative  = flag.String("red_operative", "", "Command for the red operative bot")
		blueName      = flag.String("blue_name", "blue", "Name of the blue team")
		blueSpymaster = flag.String("blue_spymaster", "", "Command for the blue spymaster bot")
		blueOperative = flag.String("blue_operative", "", "Command for the blue operative bot")
		moveTimeout   = flag.Duration("move_timeout", botproto.DefaultMoveTimeout, "Time limit for each bot move")
		maxTurns      = flag.Int("max_turns", botproto.MaxTurns, "End the game after this many turns")
		seed          = flag.Int64("seed", 0, "Board seed, to replay a game's board and starting team exactly (default: random)")
		logDir        = flag.String("log_dir", "logs", "Directory for move logs and the results file")
	)
	flag.Parse()

	red := botproto.Player{Name: *redName, Spymaster: strings.Fields(*redSpymaster), Operative: strings.Fields(*redOperative)}
	blue := botproto.Player{Name: *blueName, Spymaster: strings.Fields(*blueSpymaster), Operative: strings.Fields(*blueOperative)}
	for _, p := range []botproto.Player{red, blue} {
		if len(p.Spymaster) == 0 || len(p.Operative) == 0 {
			return fmt.Errorf("team %q needs both a spymaster and an operative command", p.Name)
		}
	}

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(*seed))
	starter := codenames.RedTeam
	if r.Intn(2) == 1 {
		starter = codenames.BlueTeam
	}
	board := boardgen.New(starter, r)

	start := time.Now()
	res, err := botproto.RunMatch(context.Background(), red, blue, board, starter, botproto.Options{
		MoveTimeout: *moveTimeout,
		MaxTurns:    *maxTurns,
	})
	if err != nil {
		return fmt.Errorf("running match: %w", err)
	}

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}
	logName := fmt.Sprintf("%s_%s-vs-%s", start.Format("2006-01-02_150405"), fileSafe(red.Name), fileSafe(blue.Name))
	logFile, err := createUnique(filepath.Join(*logDir, logName), ".jsonl")
	if err != nil {
		return fmt.Errorf("creating move log: %w", err)
	}
	logPath := logFile.Name()
	info := matchStart{
		Type:          "match_start",
		Time:          start.Format(time.RFC3339),
		Seed:          *seed,
		StartingTeam:  strings.ToLower(string(starter)),
		MoveTimeoutMS: moveTimeout.Milliseconds(),
		MaxTurns:      *maxTurns,
		Red:           red,
		Blue:          blue,
		Board:         botproto.CardsFromBoard(board, true),
	}
	if err := writeLog(logFile, info, res); err != nil {
		return fmt.Errorf("writing move log: %w", err)
	}
	if err := appendResult(filepath.Join(*logDir, "results.csv"), start, red, blue, res, logPath); err != nil {
		return fmt.Errorf("writing results: %w", err)
	}

	printSummary(info, res, logPath)
	return nil
}

type matchStart struct {
	Type          string          `json:"type"`
	Time          string          `json:"time"`
	Seed          int64           `json:"seed"`
	StartingTeam  string          `json:"starting_team"`
	MoveTimeoutMS int64           `json:"move_timeout_ms"`
	MaxTurns      int             `json:"max_turns"`
	Red           botproto.Player `json:"red"`
	Blue          botproto.Player `json:"blue"`
	Board         []botproto.Card `json:"board"`
}

// turnEvent is one clue, guess, forfeit, or turn_end line in the move log.
type turnEvent struct {
	Type     string            `json:"type"`
	Turn     int               `json:"turn"`
	Team     botproto.Team     `json:"team"`
	Role     botproto.Role     `json:"role,omitempty"`
	Bot      string            `json:"bot,omitempty"`
	Word     string            `json:"word,omitempty"`
	Count    int               `json:"count,omitempty"`
	Revealed botproto.CardType `json:"revealed,omitempty"`
	End      botproto.TurnEnd  `json:"end,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Stderr   string            `json:"stderr,omitempty"`
}

// matchResult is the last line of the move log. Winner is empty for a draw.
type matchResult struct {
	Type       string          `json:"type"`
	Winner     botproto.Team   `json:"winner"`
	WinnerName string          `json:"winner_name"`
	Reason     string          `json:"reason"`
	Turns      int             `json:"turns"`
	Forfeits   int             `json:"forfeits"`
	FinalBoard []botproto.Card `json:"final_board"`
}

// writeLog writes the match as JSONL: match_start, then per turn a clue,
// its guesses, any forfeit, and a turn_end, then the final result.
func writeLog(f *os.File, info matchStart, res *botproto.Result) error {
	var err error
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	write := func(v any) {
		if err == nil {
			err = enc.Encode(v)
		}
	}

	write(info)
	for i, t := range res.History {
		n := i + 1
		if t.Clue != nil {
			write(turnEvent{Type: "clue", Turn: n, Team: t.Team, Word: t.Clue.Word, Count: t.Clue.Count})
		}
		for _, g := range t.Guesses {
			write(turnEvent{Type: "guess", Turn: n, Team: t.Team, Word: g.Word, Revealed: g.Result})
		}
		for _, ev := range res.Events {
			if ev.Turn == n {
				write(turnEvent{Type: "forfeit", Turn: n, Team: ev.Team, Role: ev.Role, Bot: ev.Bot, Reason: ev.Error, Stderr: ev.Stderr})
			}
		}
		write(turnEvent{Type: "turn_end", Turn: n, Team: t.Team, End: t.End})
	}
	write(matchResult{
		Type:       "result",
		Winner:     res.Winner,
		WinnerName: teamName(info.Red, info.Blue, res.Winner),
		Reason:     res.Reason,
		Turns:      len(res.History),
		Forfeits:   len(res.Events),
		FinalBoard: res.Board,
	})

	return errors.Join(err, f.Close())
}

// appendResult adds one row to the results CSV, writing a header if the file
// is new.
func appendResult(path string, start time.Time, red, blue botproto.Player, res *botproto.Result, logPath string) error {
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if errors.Is(statErr, os.ErrNotExist) {
		w.Write([]string{"time", "red_team", "blue_team", "winner", "turns", "forfeits", "reason", "log_file"})
	}
	winner := teamName(red, blue, res.Winner)
	if winner == "" {
		winner = "draw"
	}
	w.Write([]string{
		start.Format(time.RFC3339), red.Name, blue.Name, winner,
		strconv.Itoa(len(res.History)), strconv.Itoa(len(res.Events)), res.Reason, filepath.Base(logPath),
	})
	w.Flush()
	return errors.Join(w.Error(), f.Close())
}

func printSummary(info matchStart, res *botproto.Result, logPath string) {
	fmt.Printf("Match:    %s (red) vs %s (blue), %s starts, seed %d\n", info.Red.Name, info.Blue.Name, info.StartingTeam, info.Seed)
	if res.Winner == "" {
		fmt.Println("Winner:   draw")
	} else {
		fmt.Printf("Winner:   %s (%s)\n", teamName(info.Red, info.Blue, res.Winner), res.Winner)
	}
	fmt.Printf("Ended by: %s\n", res.Reason)
	fmt.Printf("Turns:    %d\n", len(res.History))
	if len(res.Events) == 0 {
		fmt.Println("Forfeits: none")
	} else {
		fmt.Printf("Forfeits: %d\n", len(res.Events))
		for _, ev := range res.Events {
			fmt.Printf("  turn %d, %s %s (%s): %s\n", ev.Turn, ev.Team, ev.Role, ev.Bot, ev.Error)
		}
	}
	fmt.Printf("Log:      %s\n", logPath)
}

// createUnique creates base+ext, or base-2+ext, base-3+ext, ... if it exists,
// so matches started in the same second don't overwrite each other's logs.
func createUnique(base, ext string) (*os.File, error) {
	path := base + ext
	for i := 2; ; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if !errors.Is(err, os.ErrExist) {
			return f, err
		}
		path = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

func teamName(red, blue botproto.Player, t botproto.Team) string {
	switch t {
	case botproto.TeamRed:
		return red.Name
	case botproto.TeamBlue:
		return blue.Name
	}
	return ""
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func fileSafe(name string) string {
	if s := unsafeChars.ReplaceAllString(name, "_"); s != "" {
		return s
	}
	return "team"
}

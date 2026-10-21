package arena

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Config controls how matches are played and where their output goes.
type Config struct {
	// Seed fixes the board and starting team. 0 picks one at random.
	Seed        int64
	MoveTimeout time.Duration
	MaxTurns    int
	// LogDir gets one JSONL move log per match plus results.csv.
	LogDir string
}

// Match is a finished match.
type Match struct {
	Info    botproto.LogMatchStart
	Result  *botproto.Result
	LogPath string
}

// Winner is the winning team's name, or "" for a draw.
func (m *Match) Winner() string {
	return teamName(m.Info.Red, m.Info.Blue, m.Result.Winner)
}

// PlayMatch plays one game, writes its move log, and appends it to
// results.csv in cfg.LogDir.
func PlayMatch(ctx context.Context, red, blue botproto.Player, cfg Config) (*Match, error) {
	if cfg.MoveTimeout == 0 {
		cfg.MoveTimeout = botproto.DefaultMoveTimeout
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = botproto.MaxTurns
	}
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(seed))
	starter := codenames.RedTeam
	if r.Intn(2) == 1 {
		starter = codenames.BlueTeam
	}
	board := boardgen.New(starter, r)

	start := time.Now()
	res, err := botproto.RunMatch(ctx, red, blue, board, starter, botproto.Options{
		MoveTimeout: cfg.MoveTimeout,
		MaxTurns:    cfg.MaxTurns,
	})
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir: %w", err)
	}
	logName := fmt.Sprintf("%s_%s-vs-%s", start.Format("2006-01-02_150405"), fileSafe(red.Name), fileSafe(blue.Name))
	logFile, err := createUnique(filepath.Join(cfg.LogDir, logName), ".jsonl")
	if err != nil {
		return nil, fmt.Errorf("creating move log: %w", err)
	}
	m := &Match{
		Info: botproto.LogMatchStart{
			Type:          botproto.LogMatchStartType,
			Time:          start.Format(time.RFC3339),
			Seed:          seed,
			StartingTeam:  botproto.Team(strings.ToLower(string(starter))),
			MoveTimeoutMS: cfg.MoveTimeout.Milliseconds(),
			MaxTurns:      cfg.MaxTurns,
			Red:           red,
			Blue:          blue,
			Board:         botproto.CardsFromBoard(board, true),
		},
		Result:  res,
		LogPath: logFile.Name(),
	}
	if err := writeLog(logFile, m.Info, res); err != nil {
		return nil, fmt.Errorf("writing move log: %w", err)
	}
	if err := appendResult(filepath.Join(cfg.LogDir, "results.csv"), start, red, blue, res, m.LogPath); err != nil {
		return nil, fmt.Errorf("writing results: %w", err)
	}
	return m, nil
}

// PrintSummary writes a short human-readable summary of a match.
func PrintSummary(w io.Writer, m *Match) {
	printSummaryTo(w, m.Info, m.Result, m.LogPath)
}

// writeLog writes the match as JSONL: match_start, then per turn a clue,
// its guesses, any forfeit, and a turn_end, then the final result.
func writeLog(f *os.File, info botproto.LogMatchStart, res *botproto.Result) error {
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
			write(botproto.LogEvent{Type: botproto.LogClue, Turn: n, Team: t.Team, Word: t.Clue.Word, Count: t.Clue.Count})
		}
		for _, g := range t.Guesses {
			write(botproto.LogEvent{Type: botproto.LogGuess, Turn: n, Team: t.Team, Word: g.Word, Revealed: g.Result})
		}
		for _, ev := range res.Events {
			if ev.Turn == n {
				write(botproto.LogEvent{Type: botproto.LogForfeit, Turn: n, Team: ev.Team, Role: ev.Role, Bot: ev.Bot, Reason: ev.Error, Stderr: ev.Stderr})
			}
		}
		write(botproto.LogEvent{Type: botproto.LogTurnEnd, Turn: n, Team: t.Team, End: t.End})
	}
	write(botproto.LogResult{
		Type:       botproto.LogResultType,
		Winner:     res.Winner,
		WinnerName: teamName(info.Red, info.Blue, res.Winner),
		Reason:     res.Reason,
		Tiebreak:   res.Tiebreak,
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

func printSummaryTo(w io.Writer, info botproto.LogMatchStart, res *botproto.Result, logPath string) {
	fmt.Fprintf(w, "Match:    %s (red) vs %s (blue), %s starts, seed %d\n", info.Red.Name, info.Blue.Name, info.StartingTeam, info.Seed)
	if res.Winner == "" {
		fmt.Fprintln(w, "Winner:   draw")
	} else {
		fmt.Fprintf(w, "Winner:   %s (%s)\n", teamName(info.Red, info.Blue, res.Winner), res.Winner)
	}
	fmt.Fprintf(w, "Ended by: %s\n", res.Reason)
	fmt.Fprintf(w, "Turns:    %d\n", len(res.History))
	if len(res.Events) == 0 {
		fmt.Fprintln(w, "Forfeits: none")
	} else {
		fmt.Fprintf(w, "Forfeits: %d\n", len(res.Events))
		for _, ev := range res.Events {
			fmt.Fprintf(w, "  turn %d, %s %s (%s): %s\n", ev.Turn, ev.Team, ev.Role, ev.Bot, ev.Error)
		}
	}
	fmt.Fprintf(w, "Log:      %s\n", logPath)
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

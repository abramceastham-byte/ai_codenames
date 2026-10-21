// Binary tournament plays every pair of teams in students/ twice, swapping
// colors, and prints a standings table. Each match writes a move log and a
// results.csv row, the same as cmd/match.
//
//	go run ./cmd/tournament
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bcspragu/Codenames/arena"
	"github.com/bcspragu/Codenames/botproto"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tournament:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		studentsDir = flag.String("students_dir", "students", "Folder containing one folder per team")
		teamList    = flag.String("teams", "", "Comma-separated teams to include (default: every folder in --students_dir)")
		moveTimeout = flag.Duration("move_timeout", botproto.DefaultMoveTimeout, "Time limit for each bot move")
		maxTurns    = flag.Int("max_turns", botproto.MaxTurns, "End each game after this many turns")
		seed        = flag.Int64("seed", 0, "Base seed; pair i plays on board seed+i, to rerun a tournament's boards (default: random)")
		logDir      = flag.String("log_dir", "logs", "Directory for move logs and the results file")
	)
	flag.Parse()

	names := strings.FieldsFunc(*teamList, func(r rune) bool { return r == ',' || r == ' ' })
	if len(names) == 0 {
		var err error
		if names, err = arena.ListTeams(*studentsDir); err != nil {
			return err
		}
	}
	if len(names) < 2 {
		return fmt.Errorf("need at least 2 teams, found %d in %s", len(names), *studentsDir)
	}

	// Load every team up front, so a broken team stops the tournament before
	// any games are played.
	teams := make([]botproto.Player, len(names))
	for i, name := range names {
		p, err := arena.LoadTeam(*studentsDir, name)
		if err != nil {
			return err
		}
		teams[i] = p
	}

	base := *seed
	if base == 0 {
		base = time.Now().UnixNano()
	}
	cfg := arena.Config{MoveTimeout: *moveTimeout, MaxTurns: *maxTurns, LogDir: *logDir}

	standings := newStandings(names)
	total := len(teams) * (len(teams) - 1)
	played := 0
	fmt.Printf("Tournament: %d teams, %d matches, base seed %d\n\n", len(teams), total, base)
	for i := 0; i < len(teams); i++ {
		for j := i + 1; j < len(teams); j++ {
			// Both games of a pair use the same board, with colors swapped, so
			// each team plays the starting side once.
			cfg.Seed = base + int64(played/2)
			for _, pair := range [][2]botproto.Player{{teams[i], teams[j]}, {teams[j], teams[i]}} {
				played++
				m, err := arena.PlayMatch(context.Background(), pair[0], pair[1], cfg)
				if err != nil {
					return fmt.Errorf("match %s vs %s: %w", pair[0].Name, pair[1].Name, err)
				}
				standings.add(m)
				fmt.Printf("[%d/%d] %s\n", played, total, oneLine(m))
			}
		}
	}

	fmt.Println()
	standings.print(os.Stdout)
	fmt.Printf("\nLogs and results.csv are in %s\n", *logDir)
	return nil
}

func oneLine(m *arena.Match) string {
	vs := fmt.Sprintf("%s (red) vs %s (blue)", m.Info.Red.Name, m.Info.Blue.Name)
	outcome := "draw"
	if w := m.Winner(); w != "" {
		outcome = w + " won"
	}
	s := fmt.Sprintf("%s: %s, %s, %d turns", vs, outcome, m.Result.Reason, len(m.Result.History))
	if n := len(m.Result.Events); n > 0 {
		s += fmt.Sprintf(", %d forfeits", n)
	}
	return s
}

type record struct {
	name                       string
	wins, losses, draws, forfs int
}

type standings struct {
	byName map[string]*record
	order  []*record
}

func newStandings(names []string) *standings {
	s := &standings{byName: map[string]*record{}}
	for _, n := range names {
		r := &record{name: n}
		s.byName[n] = r
		s.order = append(s.order, r)
	}
	return s
}

func (s *standings) add(m *arena.Match) {
	red, blue := s.byName[m.Info.Red.Name], s.byName[m.Info.Blue.Name]
	switch m.Result.Winner {
	case botproto.TeamRed:
		red.wins++
		blue.losses++
	case botproto.TeamBlue:
		blue.wins++
		red.losses++
	default:
		red.draws++
		blue.draws++
	}
	for _, ev := range m.Result.Events {
		if ev.Team == botproto.TeamRed {
			red.forfs++
		} else {
			blue.forfs++
		}
	}
}

// print writes the table, ranked by wins, then draws, then fewest forfeits.
// Teams tied on all three share a rank.
func (s *standings) print(w io.Writer) {
	rows := append([]*record(nil), s.order...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.wins != b.wins {
			return a.wins > b.wins
		}
		if a.draws != b.draws {
			return a.draws > b.draws
		}
		return a.forfs < b.forfs
	})
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Rank\tTeam\tPlayed\tWins\tLosses\tDraws\tForfeits")
	rank := 0
	for i, r := range rows {
		if i == 0 || r.wins != rows[i-1].wins || r.draws != rows[i-1].draws || r.forfs != rows[i-1].forfs {
			rank = i + 1
		}
		fmt.Fprintf(tw, "%d\t%s\t%d\t%d\t%d\t%d\t%d\n", rank, r.name, r.wins+r.losses+r.draws, r.wins, r.losses, r.draws, r.forfs)
	}
	tw.Flush()
}

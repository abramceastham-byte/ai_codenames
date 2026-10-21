// Binary match plays one game between two teams of JSON bots (see
// BOT_GUIDE.md), writes a JSONL move log, and appends the outcome to a
// results file.
//
// Teams are folders in students/ (see arena.LoadTeam):
//
//	go run ./cmd/match test_student test_student_2
//
// Or give each bot's command directly:
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
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bcspragu/Codenames/arena"
	"github.com/bcspragu/Codenames/botproto"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "match:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: match [flags] <red team> <blue team>")
		fmt.Fprintln(flag.CommandLine.Output(), "   or: match [flags] --red_spymaster ... --red_operative ... --blue_spymaster ... --blue_operative ...")
		flag.PrintDefaults()
	}
	var (
		studentsDir   = flag.String("students_dir", "students", "Folder containing one folder per team")
		redName       = flag.String("red_name", "red", "Name of the red team (with --red_spymaster/--red_operative)")
		redSpymaster  = flag.String("red_spymaster", "", "Command for the red spymaster bot (split on spaces, no shell)")
		redOperative  = flag.String("red_operative", "", "Command for the red operative bot")
		blueName      = flag.String("blue_name", "blue", "Name of the blue team (with --blue_spymaster/--blue_operative)")
		blueSpymaster = flag.String("blue_spymaster", "", "Command for the blue spymaster bot")
		blueOperative = flag.String("blue_operative", "", "Command for the blue operative bot")
		moveTimeout   = flag.Duration("move_timeout", botproto.DefaultMoveTimeout, "Time limit for each bot move")
		maxTurns      = flag.Int("max_turns", botproto.MaxTurns, "End the game after this many turns")
		seed          = flag.Int64("seed", 0, "Board seed, to replay a game's board and starting team exactly (default: random)")
		logDir        = flag.String("log_dir", "logs", "Directory for move logs and the results file")
	)
	flag.Parse()

	var red, blue botproto.Player
	commandFlags := *redSpymaster + *redOperative + *blueSpymaster + *blueOperative
	switch {
	case flag.NArg() == 2 && commandFlags == "":
		var err error
		if red, err = arena.LoadTeam(*studentsDir, flag.Arg(0)); err != nil {
			return err
		}
		if blue, err = arena.LoadTeam(*studentsDir, flag.Arg(1)); err != nil {
			return err
		}
	case flag.NArg() == 0 && commandFlags != "":
		red = botproto.Player{Name: *redName, Spymaster: strings.Fields(*redSpymaster), Operative: strings.Fields(*redOperative)}
		blue = botproto.Player{Name: *blueName, Spymaster: strings.Fields(*blueSpymaster), Operative: strings.Fields(*blueOperative)}
	case flag.NArg() > 0 && commandFlags != "":
		return errors.New("give either two team names or the --red_*/--blue_* command flags, not both")
	default:
		flag.Usage()
		os.Exit(2)
	}

	m, err := arena.PlayMatch(context.Background(), red, blue, arena.Config{
		Seed:        *seed,
		MoveTimeout: *moveTimeout,
		MaxTurns:    *maxTurns,
		LogDir:      *logDir,
	})
	if err != nil {
		return err
	}
	arena.PrintSummary(os.Stdout, m)
	return nil
}

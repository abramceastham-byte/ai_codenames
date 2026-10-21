// Binary replay steps through a move log written by cmd/match.
//
// Usage:
//
//	replay [--auto] [--delay 1s] [--spymaster] [--no-color] logs/<file>.jsonl
//
// Controls: Enter = next event, b = back, s = toggle spymaster/operative
// view, q = quit.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "replay:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: replay [flags] logs/<file>.jsonl")
		fmt.Fprintln(fs.Output(), "Controls: Enter = next, b = back, s = spymaster/operative view, q = quit")
		fs.PrintDefaults()
	}
	var (
		auto      = fs.Bool("auto", false, "Play the whole game hands-free")
		delay     = fs.Duration("delay", time.Second, "Time between events with --auto")
		spymaster = fs.Bool("spymaster", false, "Start in spymaster view (all card types visible)")
		noColor   = fs.Bool("no-color", false, "Disable colors")
	)
	// Allow flags before or after the file name.
	fs.Parse(os.Args[1:])
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(2)
	}
	path := fs.Arg(0)
	fs.Parse(fs.Args()[1:])
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rp, err := loadReplay(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	outTTY := term.IsTerminal(int(os.Stdout.Fd()))
	color := outTTY && !*noColor && os.Getenv("NO_COLOR") == ""
	v := &viewer{rp: rp, spymaster: *spymaster, color: color, clear: outTTY, out: os.Stdout}

	if *auto {
		for i := range rp.frames {
			v.show(i, "")
			if i < len(rp.frames)-1 {
				time.Sleep(*delay)
			}
		}
		return nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		old, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("setting up keyboard input: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), old)
		// Raw mode turns off newline translation, so add carriage returns.
		v.raw = true
		return v.loop(keyReader(os.Stdin))
	}
	// Not a terminal (e.g. piped input): read one command per line.
	return v.loop(lineReader(os.Stdin))
}

type viewer struct {
	rp        *replay
	spymaster bool
	color     bool
	clear     bool
	raw       bool
	out       io.Writer
}

func (v *viewer) show(i int, help string) {
	s := v.rp.render(i, v.spymaster, v.color)
	if help != "" {
		s += "\n" + help + "\n"
	}
	if v.clear {
		s = "\033[H\033[2J" + s
	} else {
		s = strings.Repeat("-", 60) + "\n" + s
	}
	if v.raw {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	fmt.Fprint(v.out, s)
}

// loop shows frames and handles commands until quit or end of input.
func (v *viewer) loop(next func() (byte, error)) error {
	const help = "Enter = next · b = back · s = spymaster/operative view · q = quit"
	i := 0
	for {
		h := help
		if i == len(v.rp.frames)-1 {
			h = "End of game · b = back · s = toggle view · q = quit"
		}
		v.show(i, h)

		key, err := next()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		switch key {
		case '\r', '\n', ' ', 'n':
			if i < len(v.rp.frames)-1 {
				i++
			}
		case 'b':
			if i > 0 {
				i--
			}
		case 's':
			v.spymaster = !v.spymaster
		case 'q', 3, 4: // q, Ctrl-C, Ctrl-D
			return nil
		}
	}
}

func keyReader(r io.Reader) func() (byte, error) {
	buf := make([]byte, 1)
	return func() (byte, error) {
		_, err := r.Read(buf)
		return buf[0], err
	}
}

func lineReader(r io.Reader) func() (byte, error) {
	sc := bufio.NewScanner(r)
	return func() (byte, error) {
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			return '\n', nil
		}
		return line[0], nil
	}
}

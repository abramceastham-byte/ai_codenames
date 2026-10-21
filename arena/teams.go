// Package arena loads student teams and plays matches between them, writing
// move logs and a results file. It's shared by cmd/match and cmd/tournament.
package arena

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bcspragu/Codenames/botproto"
)

// TeamFile is the optional team.json in a team's folder. Commands are split
// on spaces and run with the team's folder as the working directory.
type TeamFile struct {
	Spymaster string `json:"spymaster"`
	Operative string `json:"operative"`
}

// LoadTeam loads the team in studentsDir/name. Without a team.json, the team
// runs spymaster.py and operative.py with python3 (or python). It returns an
// error if the folder, team.json, or any bot command can't be found.
func LoadTeam(studentsDir, name string) (botproto.Player, error) {
	dir, err := filepath.Abs(filepath.Join(studentsDir, name))
	if err != nil {
		return botproto.Player{}, err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		msg := fmt.Sprintf("team %q not found: no folder %s", name, filepath.Join(studentsDir, name))
		if names, err := ListTeams(studentsDir); err == nil {
			msg += fmt.Sprintf(" (teams in %s: %s)", studentsDir, strings.Join(names, ", "))
		}
		return botproto.Player{}, errors.New(msg)
	}

	p := botproto.Player{Name: name, Dir: dir}
	data, err := os.ReadFile(filepath.Join(dir, "team.json"))
	switch {
	case err == nil:
		var tf TeamFile
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&tf); err != nil {
			return botproto.Player{}, fmt.Errorf("team %q: invalid team.json: %w", name, err)
		}
		p.Spymaster, p.Operative = strings.Fields(tf.Spymaster), strings.Fields(tf.Operative)
		if len(p.Spymaster) == 0 || len(p.Operative) == 0 {
			return botproto.Player{}, fmt.Errorf(`team %q: team.json needs both "spymaster" and "operative" commands`, name)
		}
	case errors.Is(err, os.ErrNotExist):
		py, err := findPython()
		if err != nil {
			return botproto.Player{}, fmt.Errorf("team %q has no team.json, so it needs Python: %w", name, err)
		}
		p.Spymaster, p.Operative = []string{py, "spymaster.py"}, []string{py, "operative.py"}
	default:
		return botproto.Player{}, fmt.Errorf("team %q: reading team.json: %w", name, err)
	}

	hint := ""
	if errors.Is(err, os.ErrNotExist) {
		hint = " (a team without team.json needs spymaster.py and operative.py; for other languages, add a team.json, see BOT_GUIDE.md)"
	}
	if err := botproto.CheckCommand(p.Spymaster, dir); err != nil {
		return botproto.Player{}, fmt.Errorf("team %q spymaster: %w%s", name, err, hint)
	}
	if err := botproto.CheckCommand(p.Operative, dir); err != nil {
		return botproto.Player{}, fmt.Errorf("team %q operative: %w%s", name, err, hint)
	}
	return p, nil
}

// ListTeams returns the names of the team folders in studentsDir, sorted.
// Folders starting with "." or "_" are skipped.
func ListTeams(studentsDir string) ([]string, error) {
	entries, err := os.ReadDir(studentsDir)
	if err != nil {
		return nil, fmt.Errorf("reading teams folder: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(e.Name(), "_") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// findPython returns "python3" if it's on PATH, otherwise "python".
func findPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", errors.New("neither python3 nor python was found on PATH")
}

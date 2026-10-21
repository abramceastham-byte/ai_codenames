package arena

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTeam creates studentsDir/name with copies of the example Python bots,
// plus a team.json if teamJSON isn't empty.
func makeTeam(t *testing.T, studentsDir, name, teamJSON string) string {
	t.Helper()
	dir := filepath.Join(studentsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"spymaster.py", "operative.py"} {
		data, err := os.ReadFile(filepath.Join("..", "examples", "python", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if teamJSON != "" {
		if err := os.WriteFile(filepath.Join(dir, "team.json"), []byte(teamJSON), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func requirePython(t *testing.T) string {
	t.Helper()
	py, err := findPython()
	if err != nil {
		t.Skip(err)
	}
	return py
}

func TestLoadTeamDefault(t *testing.T) {
	py := requirePython(t)
	students := t.TempDir()
	dir := makeTeam(t, students, "alpha", "")

	p, err := LoadTeam(students, "alpha")
	if err != nil {
		t.Fatalf("LoadTeam: %v", err)
	}
	if p.Name != "alpha" || p.Dir != dir || strings.Join(p.Spymaster, " ") != py+" spymaster.py" || strings.Join(p.Operative, " ") != py+" operative.py" {
		t.Errorf("got %+v", p)
	}
}

func TestLoadTeamFile(t *testing.T) {
	py := requirePython(t)
	students := t.TempDir()
	dir := makeTeam(t, students, "beta", `{"spymaster": "`+py+` spymaster.py --verbose", "operative": "`+py+` operative.py"}`)

	p, err := LoadTeam(students, "beta")
	if err != nil {
		t.Fatalf("LoadTeam: %v", err)
	}
	if strings.Join(p.Spymaster, " ") != py+" spymaster.py --verbose" || p.Dir != dir {
		t.Errorf("got %+v", p)
	}
}

func TestLoadTeamErrors(t *testing.T) {
	py := requirePython(t)
	students := t.TempDir()
	makeTeam(t, students, "good", "")
	makeTeam(t, students, "badjson", `{"spymaster": `)
	makeTeam(t, students, "unknownfield", `{"spymaster": "x", "operative": "y", "language": "go"}`)
	makeTeam(t, students, "nooperative", `{"spymaster": "`+py+` spymaster.py"}`)
	makeTeam(t, students, "noscript", `{"spymaster": "`+py+` missing.py", "operative": "`+py+` operative.py"}`)
	makeTeam(t, students, "noprogram", `{"spymaster": "no-such-program-xyz", "operative": "`+py+` operative.py"}`)
	os.Remove(filepath.Join(makeTeam(t, students, "nooppy", ""), "operative.py"))

	tests := []struct{ team, wantErr string }{
		{"missing", `team "missing" not found`},
		{"missing", "teams in " + students + ": badjson, good, "},
		{"badjson", `team "badjson": invalid team.json`},
		{"unknownfield", `unknown field "language"`},
		{"nooperative", `team.json needs both "spymaster" and "operative"`},
		{"noscript", `team "noscript" spymaster: file "missing.py" not found`},
		{"noprogram", `program "no-such-program-xyz" not found`},
		{"nooppy", `team "nooppy" operative: file "operative.py" not found`},
	}
	for _, tt := range tests {
		_, err := LoadTeam(students, tt.team)
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("LoadTeam(%q) = %v, want error containing %q", tt.team, err, tt.wantErr)
		}
	}
}

func TestListTeams(t *testing.T) {
	students := t.TempDir()
	for _, d := range []string{"zeta", "alpha", ".git", "_template"} {
		os.Mkdir(filepath.Join(students, d), 0o755)
	}
	os.WriteFile(filepath.Join(students, "README.md"), nil, 0o644)

	got, err := ListTeams(students)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "alpha,zeta" {
		t.Errorf("ListTeams = %v, want [alpha zeta]", got)
	}
}

func TestPlayMatch(t *testing.T) {
	requirePython(t)
	students, logs := t.TempDir(), t.TempDir()
	makeTeam(t, students, "alpha", "")
	makeTeam(t, students, "beta", "")
	red, err := LoadTeam(students, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	blue, err := LoadTeam(students, "beta")
	if err != nil {
		t.Fatal(err)
	}

	m, err := PlayMatch(context.Background(), red, blue, Config{Seed: 42, LogDir: logs})
	if err != nil {
		t.Fatalf("PlayMatch: %v", err)
	}
	if len(m.Result.Events) != 0 {
		t.Errorf("example bots forfeited: %+v", m.Result.Events)
	}
	if m.Info.Seed != 42 || filepath.Dir(m.LogPath) != logs || !strings.Contains(m.LogPath, "alpha-vs-beta") {
		t.Errorf("bad match info: seed %d, log %s", m.Info.Seed, m.LogPath)
	}
	data, err := os.ReadFile(m.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !strings.Contains(lines[0], `"type":"match_start"`) || !strings.Contains(lines[len(lines)-1], `"type":"result"`) {
		t.Errorf("log should start with match_start and end with result")
	}

	f, err := os.Open(filepath.Join(logs, "results.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1][1] != "alpha" || rows[1][2] != "beta" || rows[1][7] != filepath.Base(m.LogPath) {
		t.Errorf("bad results.csv: %v", rows)
	}
}

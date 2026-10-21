package botproto

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/bcspragu/Codenames/codenames"
)

// The word list published for students must match the engine's.
func TestWordListMatchesEngine(t *testing.T) {
	data, err := os.ReadFile("../examples/wordlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(data))
	want := slices.Clone(codenames.Words)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("examples/wordlist.txt is out of date: got %d words, want %d (regenerate it from codenames/words.go)", len(got), len(want))
	}
}

// The sample requests for students must be valid, complete requests.
func TestSampleRequests(t *testing.T) {
	for _, tt := range []struct {
		file string
		role Role
	}{
		{"../examples/sample_spymaster_request.json", RoleSpymaster},
		{"../examples/sample_operative_request.json", RoleOperative},
	} {
		data, err := os.ReadFile(tt.file)
		if err != nil {
			t.Fatal(err)
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		var req Request
		if err := dec.Decode(&req); err != nil {
			t.Fatalf("%s: %v", tt.file, err)
		}
		if req.ProtocolVersion != ProtocolVersion || req.Role != tt.role || req.StartingTeam == "" || len(req.Board) != codenames.Size {
			t.Errorf("%s: bad request header or board size: %+v", tt.file, req)
		}
		for _, c := range req.Board {
			hidden := c.Type == ""
			if tt.role == RoleSpymaster && hidden {
				t.Errorf("%s: spymaster card %q has no type", tt.file, c.Word)
			}
			if tt.role == RoleOperative && hidden == c.Revealed {
				t.Errorf("%s: operative card %q: revealed=%t but type=%q", tt.file, c.Word, c.Revealed, c.Type)
			}
		}
		if tt.role == RoleOperative && (req.Clue == nil || req.GuessesAllowed != req.Clue.Count+1) {
			t.Errorf("%s: operative needs a clue and guesses_allowed = count+1", tt.file)
		}
	}
}

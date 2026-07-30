package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bcspragu/Codenames/codenames"
	"github.com/bcspragu/Codenames/memdb"
	"github.com/bcspragu/Codenames/msgs"
	"github.com/google/go-cmp/cmp"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
)

func TestBasicallyEverything(t *testing.T) {
	// This is a hodge-podge test that tests out the entire flow end-to-end,
	// because this is a personal project and I don't have the wherewithal to add
	// more modular tests.
	env := setup()

	for i := range 5 {
		env.createUser(t, fmt.Sprintf("Test%d", i))
	}

	// Sanity check the auth works by requesting a users information back.
	gotUser := env.user(t, 3 /* user index 3 */)
	wantUser := &codenames.User{
		ID:   "user_3",
		Name: "Test3",
	}
	if diff := cmp.Diff(wantUser, gotUser); diff != "" {
		t.Errorf("unexpected user (-want +got)\n%s", diff)
	}

	gID := env.createGame(t, 1)
	gotGame, err := env.db.Game(gID)
	if err != nil {
		t.Fatalf("failed to load game %q: %v", gID, err)
	}
	wantGame := &codenames.Game{
		ID:        "game_0",
		CreatedBy: "user_1",
		Status:    codenames.Pending,
		State: &codenames.GameState{
			ActiveTeam:   codenames.BlueTeam,
			ActiveRole:   codenames.SpymasterRole,
			Board:        &codenames.Board{Cards: startingBoardCards()},
			StartingTeam: codenames.BlueTeam,
			Clues:        []codenames.SpymasterClue{},
		},
	}
	if diff := cmp.Diff(wantGame, gotGame); diff != "" {
		t.Errorf("unexpected game (-want +got)\n%s", diff)
	}

	gotPendingGames := env.pendingGames(t)
	wantPendingGames := []codenames.GameID{"game_0"}
	if diff := cmp.Diff(wantPendingGames, gotPendingGames); diff != "" {
		t.Errorf("unexpected pending game IDs (-want +got)\n%s", diff)
	}

	checkPlayers := func(wantPlayers []*msgs.Player) {
		gotPlayers := env.players(t, gID, 0)
		if diff := cmp.Diff(wantPlayers, gotPlayers); diff != "" {
			t.Errorf("expected players in game (-want +got)\n%s", diff)
		}
	}

	// The creator (user_1) is auto-joined when creating the game.
	checkPlayers([]*msgs.Player{
		&msgs.Player{PlayerID: human("user_1"), Name: "Test1"},
	})

	// Have four players join that game.
	for i := 0; i < 4; i++ {
		env.joinGame(t, gID, i)
	}

	// Now, we expect everyone in, but nobody has a role.
	// user_1 is first because createGame auto-joins the creator.
	checkPlayers([]*msgs.Player{
		&msgs.Player{PlayerID: human("user_1"), Name: "Test1"},
		&msgs.Player{PlayerID: human("user_0"), Name: "Test0"},
		&msgs.Player{PlayerID: human("user_2"), Name: "Test2"},
		&msgs.Player{PlayerID: human("user_3"), Name: "Test3"},
	})

	// Have the game creator assign roles.
	assignRole := func(idx int, role codenames.Role, team codenames.Team) {
		env.assignRole(t, gID, 1 /* creator index */, fmt.Sprintf("user_%d", idx), role, team)
	}

	assignRole(0, codenames.SpymasterRole, codenames.BlueTeam)
	assignRole(1, codenames.SpymasterRole, codenames.RedTeam)
	assignRole(2, codenames.OperativeRole, codenames.BlueTeam)
	assignRole(3, codenames.OperativeRole, codenames.RedTeam)

	// Now, we expect everyone has a role.
	checkPlayers([]*msgs.Player{
		&msgs.Player{
			PlayerID: human("user_1"),
			Name:     "Test1",
			Role:     codenames.SpymasterRole,
			Team:     codenames.RedTeam,
		},
		&msgs.Player{
			PlayerID: human("user_0"),
			Name:     "Test0",
			Role:     codenames.SpymasterRole,
			Team:     codenames.BlueTeam,
		},
		&msgs.Player{
			PlayerID: human("user_2"),
			Name:     "Test2",
			Role:     codenames.OperativeRole,
			Team:     codenames.BlueTeam,
		},
		&msgs.Player{
			PlayerID: human("user_3"),
			Name:     "Test3",
			Role:     codenames.OperativeRole,
			Team:     codenames.RedTeam,
		},
	})

	// Have the game creator start the game.
	env.startGame(t, gID, 1)
}

func TestClueIsHeldUntilTheCluePhaseRunsItsMinimum(t *testing.T) {
	const delay = 500 * time.Millisecond
	env := setup(WithClueDelay(delay))
	gID := env.startedGame(t)

	// The blue spymaster answers instantly, the way an AI would.
	releaseAt, err := env.giveClue(t, gID, 0 /* user_0, blue spymaster */, "SPY", 2)
	if err != nil {
		t.Fatalf("failed to give clue: %v", err)
	}

	if until := time.Until(time.UnixMilli(releaseAt)); until <= 0 || until > delay {
		t.Errorf("clue release is %v away, want somewhere in (0, %v]", until, delay)
	}

	// Nothing may have reached the stored state yet — an operative refetching
	// the game right now must not find the clue.
	if got := env.clues(t, gID); len(got) != 0 {
		t.Fatalf("clue was published immediately: %+v", got)
	}
	g, err := env.db.Game(gID)
	if err != nil {
		t.Fatalf("failed to load game %q: %v", gID, err)
	}
	if g.State.ActiveRole != codenames.SpymasterRole {
		t.Errorf("active role is %q during the hold, want %q — the turn moved on early", g.State.ActiveRole, codenames.SpymasterRole)
	}

	// ...and once the phase has run its length, the clue comes out.
	deadline := time.Now().Add(10 * time.Second)
	for len(env.clues(t, gID)) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("clue was never released")
		}
		time.Sleep(10 * time.Millisecond)
	}

	wantClues := []codenames.SpymasterClue{
		{Clue: codenames.Clue{Word: "SPY", Count: 2}, Team: codenames.BlueTeam},
	}
	if diff := cmp.Diff(wantClues, env.clues(t, gID)); diff != "" {
		t.Errorf("unexpected clues after release (-want +got)\n%s", diff)
	}

	g, err = env.db.Game(gID)
	if err != nil {
		t.Fatalf("failed to load game %q: %v", gID, err)
	}
	if g.State.ActiveRole != codenames.OperativeRole {
		t.Errorf("active role is %q after release, want %q", g.State.ActiveRole, codenames.OperativeRole)
	}
}

func TestIllegalClueIsRejectedWithoutWaiting(t *testing.T) {
	env := setup(WithClueDelay(time.Hour))
	gID := env.startedGame(t)

	// DOCTORS is a form of the board word "doctor", so it's illegal. The
	// spymaster has to hear about that now, not in an hour.
	if _, err := env.giveClue(t, gID, 0, "DOCTORS", 1); err == nil {
		t.Fatal("giving a clue that collides with a board word succeeded, want an error")
	}

	// A rejected clue mustn't occupy the game's clue slot.
	if _, err := env.giveClue(t, gID, 0, "SPY", 1); err != nil {
		t.Fatalf("failed to give a legal clue after a rejected one: %v", err)
	}
}

func TestSecondClueIsRejectedWhileOneIsHeld(t *testing.T) {
	// A delay long enough that the first clue stays held for the whole test.
	env := setup(WithClueDelay(time.Hour))
	gID := env.startedGame(t)

	if _, err := env.giveClue(t, gID, 0, "SPY", 2); err != nil {
		t.Fatalf("failed to give clue: %v", err)
	}

	if _, err := env.giveClue(t, gID, 0, "MEDICAL", 1); err == nil {
		t.Fatal("gave a second clue while the first was still held, want an error")
	}

	if got := env.clues(t, gID); len(got) != 0 {
		t.Errorf("clues were published during the hold: %+v", got)
	}
}

func TestSwitchingTheHoldOffReleasesAHeldClue(t *testing.T) {
	// Long enough that the clue would still be waiting if the toggle did
	// nothing, so a pass can only mean the toggle released it.
	env := setup(WithClueDelay(time.Hour))
	gID := env.startedGame(t)

	team := env.state(t, gID).ActiveTeam
	spymaster := 0
	if team == codenames.RedTeam {
		spymaster = 1
	}
	if _, err := env.giveClue(t, gID, spymaster, "SPY", 2); err != nil {
		t.Fatalf("failed to give clue: %v", err)
	}
	if got := env.clues(t, gID); len(got) != 0 {
		t.Fatalf("clue was published immediately: %+v", got)
	}

	// user_1 created the game, so it's user_1 who can flip the switch.
	hold := env.setClueHold(t, gID, 1, false)
	if hold.Enabled {
		t.Errorf("clue hold reports enabled=%t after being switched off", hold.Enabled)
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(env.clues(t, gID)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("held clue was not released after the hold was switched off")
		}
		time.Sleep(10 * time.Millisecond)
	}

	wantClues := []codenames.SpymasterClue{
		{Clue: codenames.Clue{Word: "SPY", Count: 2}, Team: team},
	}
	if diff := cmp.Diff(wantClues, env.clues(t, gID)); diff != "" {
		t.Errorf("unexpected clues after the hold was switched off (-want +got)\n%s", diff)
	}
}

func TestClueGoesStraightOutWhileTheHoldIsOff(t *testing.T) {
	env := setup(WithClueDelay(time.Hour))
	gID := env.startedGame(t)

	env.setClueHold(t, gID, 1, false)

	team := env.state(t, gID).ActiveTeam
	spymaster := 0
	if team == codenames.RedTeam {
		spymaster = 1
	}
	release, err := env.giveClue(t, gID, spymaster, "SPY", 2)
	if err != nil {
		t.Fatalf("failed to give clue: %v", err)
	}
	if until := time.Until(time.UnixMilli(release)); until > 0 {
		t.Errorf("release is %v away with the hold off, want no wait", until)
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(env.clues(t, gID)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("clue was withheld even though the hold was off")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOnlyTheCreatorCanReadOrFlipTheHold(t *testing.T) {
	env := setup(WithClueDelay(time.Hour))
	gID := env.startedGame(t)

	// user_1 created the game; user_0, user_2 and user_3 did not.
	if _, err := env.clueHold(t, gID, 1); err != nil {
		t.Errorf("creator could not read the clue hold: %v", err)
	}
	for _, idx := range []int{0, 2, 3} {
		if _, err := env.clueHold(t, gID, idx); err == nil {
			t.Errorf("non-creator user_%d was able to read the clue hold", idx)
		}
		if _, err := env.setClueHoldErr(t, gID, idx, false); err == nil {
			t.Errorf("non-creator user_%d was able to switch the clue hold off", idx)
		}
	}

	// And the failed attempts left the hold alone.
	hold, err := env.clueHold(t, gID, 1)
	if err != nil {
		t.Fatalf("failed to read the clue hold: %v", err)
	}
	if !hold.Enabled {
		t.Error("clue hold was switched off by a non-creator")
	}
}

func TestTheHoldCanBeSwitchedBackOnMidGame(t *testing.T) {
	env := setup(WithClueDelay(time.Hour))
	gID := env.startedGame(t)

	env.setClueHold(t, gID, 1, false)
	if hold := env.setClueHold(t, gID, 1, true); !hold.Enabled {
		t.Fatal("clue hold would not switch back on")
	}

	team := env.state(t, gID).ActiveTeam
	spymaster := 0
	if team == codenames.RedTeam {
		spymaster = 1
	}
	if _, err := env.giveClue(t, gID, spymaster, "SPY", 2); err != nil {
		t.Fatalf("failed to give clue: %v", err)
	}
	if got := env.clues(t, gID); len(got) != 0 {
		t.Errorf("clue was published immediately after the hold was switched back on: %+v", got)
	}
}

// human returns the PlayerID as it should appear on the wire in
// player-facing messages, where the human/robot distinction is stripped to
// avoid revealing which players are AI.
func human(uID codenames.UserID) codenames.PlayerID {
	return sanitizePlayerID(uID.AsPlayerID())
}

// startingBoardCards returns the cards we expect on the test board, since we
// use a deterministic pseudo-random number generator.
func startingBoardCards() []codenames.Card {
	return []codenames.Card{
		{Codename: "dwarf", Agent: codenames.Bystander},
		{Codename: "green", Agent: codenames.Bystander},
		{Codename: "doctor", Agent: codenames.BlueAgent},
		{Codename: "ship", Agent: codenames.RedAgent},
		{Codename: "dance", Agent: codenames.Bystander},
		{Codename: "time", Agent: codenames.RedAgent},
		{Codename: "pool", Agent: codenames.BlueAgent},
		{Codename: "cover", Agent: codenames.Bystander},
		{Codename: "fighter", Agent: codenames.RedAgent},
		{Codename: "horse", Agent: codenames.RedAgent},
		{Codename: "strike", Agent: codenames.BlueAgent},
		{Codename: "cast", Agent: codenames.RedAgent},
		{Codename: "string", Agent: codenames.Bystander},
		{Codename: "greece", Agent: codenames.BlueAgent},
		{Codename: "fence", Agent: codenames.BlueAgent},
		{Codename: "drill", Agent: codenames.BlueAgent},
		{Codename: "button", Agent: codenames.Assassin},
		{Codename: "cycle", Agent: codenames.RedAgent},
		{Codename: "chest", Agent: codenames.RedAgent},
		{Codename: "pitch", Agent: codenames.Bystander},
		{Codename: "unicorn", Agent: codenames.BlueAgent},
		{Codename: "agent", Agent: codenames.BlueAgent},
		{Codename: "kiwi", Agent: codenames.Bystander},
		{Codename: "swing", Agent: codenames.RedAgent},
		{Codename: "skyscraper", Agent: codenames.BlueAgent},
	}
}

func (env *testEnv) createUser(t *testing.T, name string) {
	req := struct {
		Name string `json:"name"`
	}{name}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/user", toBody(t, req))
	if err := env.srv.serveCreateUser(w, r); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	auth := w.Header().Get("Set-Cookie")
	if auth == "" {
		t.Fatal("no auth was provided in create user response")
	}
	header := http.Header{}
	header.Add("Cookie", auth)
	request := http.Request{Header: header}
	parsedCookies := request.Cookies()
	for _, c := range parsedCookies {
		if c.Name == "Authorization" {
			env.userAuth = append(env.userAuth, c)
		}
	}
}

func (env *testEnv) user(t *testing.T, authIdx int) *codenames.User {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/user", nil)
	env.addAuth(r, authIdx)

	if err := env.srv.serveUser(w, r); err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	var u codenames.User
	fromBody(t, w, &u)
	return &u
}

func (env *testEnv) createGame(t *testing.T, authIdx int) codenames.GameID {
	req := struct {
		Private bool `json:"private"`
	}{false}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/game", toBody(t, req))
	env.addAuth(r, authIdx)

	if err := env.srv.serveCreateGame(w, r); err != nil {
		t.Fatalf("failed to create game: %v", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	fromBody(t, w, &resp)
	return codenames.GameID(resp.ID)
}

func (env *testEnv) pendingGames(t *testing.T) []codenames.GameID {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/games", nil)

	if err := env.srv.servePendingGames(w, r); err != nil {
		t.Fatalf("failed to get pending games: %v", err)
	}

	var resp []codenames.GameID
	fromBody(t, w, &resp)
	return resp
}

func (env *testEnv) joinGame(t *testing.T, gID codenames.GameID, authIdx int) {
	req := struct {
		PlayerType string `json:"player_type"`
	}{"HUMAN"}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/game/"+string(gID)+"/join", toBody(t, req))
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveJoinGame, isGamePending())
	if err := handler(w, r); err != nil {
		t.Fatalf("failed to join game: %v", err)
	}
}

func (env *testEnv) players(t *testing.T, gID codenames.GameID, authIdx int) []*msgs.Player {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/game/"+string(gID)+"/players", nil)
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveGamePlayers)
	if err := handler(w, r); err != nil {
		t.Fatalf("failed to get players: %v", err)
	}

	var resp []*msgs.Player
	fromBody(t, w, &resp)
	return resp
}

func (env *testEnv) assignRole(t *testing.T, gID codenames.GameID, authIdx int, userID string, role codenames.Role, team codenames.Team) {
	pID := codenames.PlayerID{
		PlayerType: codenames.PlayerTypeHuman,
		ID:         userID,
	}
	req := struct {
		PlayerID codenames.PlayerID `json:"player_id"`
		Team     string             `json:"team"`
		Role     string             `json:"role"`
	}{pID, string(team), string(role)}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/game/"+string(gID)+"/assignRole", toBody(t, req))
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveAssignRole, isGamePending())
	if err := handler(w, r); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}
}

func (env *testEnv) startGame(t *testing.T, gID codenames.GameID, authIdx int) {
	req := struct {
		RandomAssignment bool `json:"random_assignment"`
	}{false}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/game/"+string(gID)+"/start", toBody(t, req))
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveStartGame, isGameCreator(), isGamePending())
	if err := handler(w, r); err != nil {
		t.Fatalf("failed to start game: %v", err)
	}
}

// giveClue submits a clue and returns when the server says the operatives will
// see it, as Unix milliseconds. The handler error is returned rather than
// fataled on, because rejecting a clue is a case worth testing.
func (env *testEnv) giveClue(t *testing.T, gID codenames.GameID, authIdx int, word string, count int) (int64, error) {
	req := struct {
		Word  string `json:"word"`
		Count int    `json:"count"`
	}{word, count}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/game/"+string(gID)+"/clue", toBody(t, req))
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveClue, isSpymaster(), isGamePlaying())
	if err := handler(w, r); err != nil {
		return 0, err
	}

	var resp struct {
		Success     bool  `json:"success"`
		ReleaseAtMS int64 `json:"release_at_ms"`
	}
	fromBody(t, w, &resp)
	if !resp.Success {
		t.Fatalf("clue %q was not accepted", word)
	}
	return resp.ReleaseAtMS, nil
}

// startedGame runs a game through setup and returns its ID, with roles fixed
// as: user_0 blue spymaster, user_1 red spymaster (and game creator), user_2
// blue operative, user_3 red operative. Blue moves first.
func (env *testEnv) startedGame(t *testing.T) codenames.GameID {
	for i := range 4 {
		env.createUser(t, fmt.Sprintf("Test%d", i))
	}

	gID := env.createGame(t, 1)
	for i := range 4 {
		env.joinGame(t, gID, i)
	}

	env.assignRole(t, gID, 1, "user_0", codenames.SpymasterRole, codenames.BlueTeam)
	env.assignRole(t, gID, 1, "user_1", codenames.SpymasterRole, codenames.RedTeam)
	env.assignRole(t, gID, 1, "user_2", codenames.OperativeRole, codenames.BlueTeam)
	env.assignRole(t, gID, 1, "user_3", codenames.OperativeRole, codenames.RedTeam)

	env.startGame(t, gID, 1)

	return gID
}

// clues returns the clues recorded in the game's stored state, which is what
// an operative would see if they refetched the game.
func (env *testEnv) clues(t *testing.T, gID codenames.GameID) []codenames.SpymasterClue {
	return env.state(t, gID).Clues
}

func (env *testEnv) state(t *testing.T, gID codenames.GameID) *codenames.GameState {
	g, err := env.db.Game(gID)
	if err != nil {
		t.Fatalf("failed to load game %q: %v", gID, err)
	}
	return g.State
}

// clueHold reads the game's clue hold setting, returning the handler error so
// tests can check that non-creators are turned away.
func (env *testEnv) clueHold(t *testing.T, gID codenames.GameID, authIdx int) (clueHoldResp, error) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/game/"+string(gID)+"/clueHold", nil)
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveClueHold, isGameCreator())
	if err := handler(w, r); err != nil {
		return clueHoldResp{}, err
	}

	var resp clueHoldResp
	fromBody(t, w, &resp)
	return resp, nil
}

func (env *testEnv) setClueHoldErr(t *testing.T, gID codenames.GameID, authIdx int, on bool) (clueHoldResp, error) {
	req := struct {
		Enabled bool `json:"enabled"`
	}{on}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/game/"+string(gID)+"/clueHold", toBody(t, req))
	r = mux.SetURLVars(r, map[string]string{"id": string(gID)})
	env.addAuth(r, authIdx)

	handler := env.srv.requireGameAuth(env.srv.serveSetClueHold, isGameCreator())
	if err := handler(w, r); err != nil {
		return clueHoldResp{}, err
	}

	var resp clueHoldResp
	fromBody(t, w, &resp)
	return resp, nil
}

func (env *testEnv) setClueHold(t *testing.T, gID codenames.GameID, authIdx int, on bool) clueHoldResp {
	resp, err := env.setClueHoldErr(t, gID, authIdx, on)
	if err != nil {
		t.Fatalf("failed to set clue hold to %t: %v", on, err)
	}
	return resp
}

func (env *testEnv) addAuth(r *http.Request, authIdx int) {
	r.AddCookie(env.userAuth[authIdx])
}

func toBody(t *testing.T, body any) io.Reader {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}
	return &buf
}

func fromBody(t *testing.T, w *httptest.ResponseRecorder, resp interface{}) {
	if err := json.NewDecoder(w.Body).Decode(resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
}

type testEnv struct {
	db       *memdb.DB
	srv      *Srv
	userAuth []*http.Cookie
}

func setup(opts ...Option) *testEnv {
	db := memdb.New()

	srv := New(
		db,
		rand.New(rand.NewSource(0)),
		setupCookies(),
		nil, /* AI client, not used yet */
		"",  /* logDir, not used in tests */
		"",  /* adminSecret, not used in tests */
		opts...,
	)

	// The server assigns every player a generated name. Make them
	// deterministic ("Test0", "Test1", ...) so expectations are stable.
	var nameCount int
	srv.genName = func() string {
		name := fmt.Sprintf("Test%d", nameCount)
		nameCount++
		return name
	}

	return &testEnv{
		db:  db,
		srv: srv,
	}
}

func setupCookies() *securecookie.SecureCookie {
	return securecookie.New(
		[]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			9, 10, 11, 12, 13, 14, 15, 16,
			17, 18, 19, 20, 21, 22, 23, 24,
			25, 26, 27, 28, 29, 30, 31, 32,
		},
		[]byte{
			33, 34, 35, 36, 37, 38, 39, 40,
			41, 42, 43, 44, 45, 46, 47, 48,
			49, 50, 51, 52, 53, 54, 55, 56,
			57, 58, 59, 60, 61, 62, 63, 64,
		})
}

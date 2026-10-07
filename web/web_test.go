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

func TestIllegalClueIsRejected(t *testing.T) {
	env := setup()
	gID := env.startedGame(t)

	// DOCTORS is a form of the board word "doctor", so it's illegal.
	if err := env.giveClue(t, gID, 0, "DOCTORS", 1); err == nil {
		t.Fatal("giving a clue that collides with a board word succeeded, want an error")
	}

	// A rejected clue must leave the turn where it was, so the spymaster can
	// try again.
	if got := env.clues(t, gID); len(got) != 0 {
		t.Errorf("a rejected clue was recorded: %+v", got)
	}
	if role := env.state(t, gID).ActiveRole; role != codenames.SpymasterRole {
		t.Errorf("active role is %q after a rejected clue, want %q", role, codenames.SpymasterRole)
	}

	if err := env.giveClue(t, gID, 0, "SPY", 1); err != nil {
		t.Fatalf("failed to give a legal clue after a rejected one: %v", err)
	}
}

func TestAcceptedClueGoesStraightOut(t *testing.T) {
	env := setup()
	gID := env.startedGame(t)

	// Blue moves first in startedGame, and user_0 is their spymaster.
	if err := env.giveClue(t, gID, 0, "SPY", 2); err != nil {
		t.Fatalf("failed to give clue: %v", err)
	}

	wantClues := []codenames.SpymasterClue{
		{Clue: codenames.Clue{Word: "SPY", Count: 2}, Team: codenames.BlueTeam},
	}
	if diff := cmp.Diff(wantClues, env.clues(t, gID)); diff != "" {
		t.Errorf("unexpected clues after the clue was given (-want +got)\n%s", diff)
	}
	if role := env.state(t, gID).ActiveRole; role != codenames.OperativeRole {
		t.Errorf("active role is %q after a clue, want %q", role, codenames.OperativeRole)
	}
}

// human returns the PlayerID as it should appear on the wire for a human
// player. The study build stripped the human/robot distinction here so players
// couldn't tell which opponents were AI; a competition reports it as-is.
func human(uID codenames.UserID) codenames.PlayerID {
	return uID.AsPlayerID()
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
func (env *testEnv) giveClue(t *testing.T, gID codenames.GameID, authIdx int, word string, count int) error {
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
		return err
	}

	var resp struct {
		Success bool `json:"success"`
	}
	fromBody(t, w, &resp)
	if !resp.Success {
		t.Fatalf("clue %q was not accepted", word)
	}
	return nil
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
		opts...,
	)

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

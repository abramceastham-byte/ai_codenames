package sqldb

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/bcspragu/Codenames/codenames"

	_ "github.com/mattn/go-sqlite3"
)

var (
	// Game statements
	createGameStmt      = `INSERT INTO Games (id, status, creator_id, state) VALUES (?, ?, ?, ?)`
	gameExistsStmt      = `SELECT EXISTS(SELECT 1 FROM Games WHERE id = ?)`
	getGameStmt         = `SELECT id, status, creator_id, state FROM Games WHERE id = ?`
	getPendingGamesStmt = `SELECT id FROM Games WHERE status = 'PENDING' ORDER BY id`
	startGameStmt       = `
UPDATE Games
SET status = 'PLAYING'
WHERE id = ?`
	updateGameStateStmt = `
UPDATE Games
SET state = ?
WHERE id = ?`

	// User statements
	createUserStmt = `INSERT INTO Users (id, display_name) VALUES (?, ?)`
	getUserStmt    = `SELECT id, display_name FROM Users WHERE id = ?`

	// Robot statements
	createAIStmt = `INSERT INTO AIs (id, display_name) VALUES (?, ?)`
	getRobotStmt = `SELECT id, display_name FROM AIs WHERE id = ?`

	// Player (e.g. user or AI) statements
	getUserPlayerStmt = `SELECT id FROM Players WHERE user_id = ?`
	getAIPlayerStmt   = `SELECT id FROM Players WHERE ai_id = ?`
	createPlayerStmt  = `INSERT INTO Players (id, user_id, ai_id) VALUES (?, ?, ?)`

	// Game player (e.g. Game <-> Player join table) statements
	joinGameStmt = `
INSERT INTO GamePlayers
(game_id, player_id, role_assigned) VALUES
(?, ?, 0)`

	assignRoleStmt = `
UPDATE GamePlayers
SET role_assigned = 1,
		role = ?,
		team = ?
WHERE game_id = ?
	AND player_id = ?`
	getGamePlayers = `
SELECT Players.user_id, Players.ai_id, GamePlayers.role, GamePlayers.team, GamePlayers.role_assigned
FROM GamePlayers
JOIN Players
	ON GamePlayers.player_id = Players.id
WHERE GamePlayers.game_id = ?`

	// Game history statements (currently unused)
	updateGameHistoryStmt = `INSERT INTO GameHistory (game_id, event) VALUES (?, ?)`
)

// DB implements the Codenames database API, backed by a SQLite database.
// NOTE: Since the database doesn't support concurrent writers, we don't
// actually hold the *sql.DB in this struct, we force all callers to get a
// handle via channels.
type DB struct {
	sdb      *sql.DB
	doneChan chan struct{}
	closeFn  func() error
	r        *rand.Rand
}

// New creates a new *DB that is stored on disk at the given filename.
func New(fn string, r *rand.Rand) (*DB, error) {
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return nil, errors.New("DB needs to be initialized")
	}
	sdb, err := sql.Open("sqlite3", fn+"?_loc=UTC")
	if err != nil {
		return nil, err
	}

	// See https://briandouglas.ie/sqlite-defaults/
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 1000",
		"PRAGMA cache_size = -5000",
		"PRAGMA foreign_keys = ON",
		// We don't really delete anything
		"PRAGMA auto_vacuum = FULL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 268435456",
		"PRAGMA page_size = 4096",
	}

	for _, pragma := range pragmas {
		if _, err := sdb.Exec(pragma); err != nil {
			return nil, fmt.Errorf("failed to set pragma %q: %w", pragma, err)
		}
	}
	sdb.SetMaxOpenConns(1)

	db := &DB{
		sdb:      sdb,
		doneChan: make(chan struct{}),
		closeFn: func() error {
			return sdb.Close()
		},
		r: r,
	}
	return db, nil
}

func (s *DB) Close() error {
	return s.sdb.Close()
}

func (s *DB) NewGame(g *codenames.Game) (codenames.GameID, error) {
	gsb, err := gameStateBytes(g.State)
	if err != nil {
		return "", fmt.Errorf("failed to serialize game state: %w", err)
	}

	tx, err := s.sdb.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	id, err := s.uniqueID(tx)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(createGameStmt, string(id), codenames.Pending, string(g.CreatedBy), gsb)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return id, nil
}

func (s *DB) Game(gID codenames.GameID) (*codenames.Game, error) {
	tx, err := s.sdb.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var (
		g   codenames.Game
		gsb []byte
	)
	if err := tx.QueryRow(getGameStmt, string(gID)).Scan(&g.ID, &g.Status, &g.CreatedBy, &gsb); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if g.State, err = gameStateFromBytes(gsb); err != nil {
		return nil, err
	}

	return &g, nil
}

func (s *DB) withTx(fn func(*sql.Tx) error) error {
	tx, err := s.sdb.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *DB) NewUser(name string) (codenames.UserID, error) {
	id := codenames.RandomUserID(s.r)

	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(createUserStmt, string(id), name); err != nil {
			return err
		}

		if _, err := s.createPlayer(codenames.PlayerID{PlayerType: codenames.PlayerTypeHuman, ID: string(id)}, tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return id, nil
}

func (s *DB) NewRobot(name string) (codenames.RobotID, error) {
	id := codenames.RandomRobotID(s.r)

	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(createAIStmt, string(id), name); err != nil {
			return err
		}

		if _, err := s.createPlayer(codenames.PlayerID{PlayerType: codenames.PlayerTypeRobot, ID: string(id)}, tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return id, nil
}

func (s *DB) User(id codenames.UserID) (*codenames.User, error) {

	var u codenames.User
	err := s.sdb.QueryRow(getUserStmt, string(id)).Scan(&u.ID, &u.Name)
	if err == sql.ErrNoRows {
		return nil, codenames.ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	return &u, nil
}

func (s *DB) Robot(id codenames.RobotID) (*codenames.Robot, error) {
	var r codenames.Robot
	err := s.sdb.QueryRow(getRobotStmt, string(id)).Scan(&r.ID, &r.Name)
	if err == sql.ErrNoRows {
		return nil, codenames.ErrRobotNotFound
	} else if err != nil {
		return nil, err
	}

	return &r, nil
}

func (s *DB) PendingGames() ([]codenames.GameID, error) {
	rows, err := s.sdb.Query(getPendingGamesStmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []codenames.GameID
	for rows.Next() {
		var id codenames.GameID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

func (s *DB) PlayersInGame(gID codenames.GameID) ([]*codenames.PlayerRole, error) {
	rows, err := s.sdb.Query(getGamePlayers, gID)
	if err != nil {
		return nil, fmt.Errorf("failed to query for game players: %w", err)
	}
	defer rows.Close()

	var prs []*codenames.PlayerRole
	for rows.Next() {
		var (
			pr codenames.PlayerRole

			role   sql.NullString
			team   sql.NullString
			userID sql.NullString
			aiID   sql.NullString
		)
		if err := rows.Scan(&userID, &aiID, &role, &team, &pr.RoleAssigned); err != nil {
			return nil, fmt.Errorf("failed to scan game player: %w", err)
		}
		if role.Valid {
			pr.Role = codenames.Role(role.String)
		}
		if team.Valid {
			pr.Team = codenames.Team(team.String)
		}
		if userID.Valid && aiID.Valid {
			return nil, fmt.Errorf("both user_id and ai_id were set: %q, %q", userID.String, aiID.String)
		}
		if !userID.Valid && !aiID.Valid {
			return nil, errors.New("neither of user_id or ai_id were set")
		}
		if userID.Valid {
			pr.PlayerID = codenames.PlayerID{
				PlayerType: codenames.PlayerTypeHuman,
				ID:         userID.String,
			}
		}
		if aiID.Valid {
			pr.PlayerID = codenames.PlayerID{
				PlayerType: codenames.PlayerTypeRobot,
				ID:         aiID.String,
			}
		}
		prs = append(prs, &pr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error scanning rows: %w", err)
	}

	return prs, nil
}

func (s *DB) JoinGame(gID codenames.GameID, pID codenames.PlayerID) error {
	// First, see if a player entity already exists for this player.
	entityID, err := s.Player(pID)
	if err != nil {
		return fmt.Errorf("failed to load player: %w", err)
	}

	// If we're here, we've got a player ID and we can add them to the game.
	if _, err := s.sdb.Exec(joinGameStmt, gID, entityID); err != nil {
		return err
	}
	return nil
}

func (s *DB) AssignRole(gID codenames.GameID, req *codenames.PlayerRole) error {
	// First, see if a player entity already exists for this player.
	pID, err := s.Player(req.PlayerID)
	if err != nil {
		return fmt.Errorf("failed to load player: %w", err)
	}

	// If we're here, we've got a player ID and we can add them to the game.
	res, err := s.sdb.Exec(assignRoleStmt, string(req.Role), string(req.Team), gID, pID)
	if err != nil {
		return fmt.Errorf("failed to assign role: %w", err)
	}
	numRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get the number of affected rows: %w", err)
	}
	if numRows != 1 {
		return fmt.Errorf("%d rows affected, expected exactly 1", numRows)
	}
	return nil
}

func (s *DB) createPlayer(id codenames.PlayerID, tx *sql.Tx) (string, error) {
	pID := codenames.RandomPlayerID(s.r)
	var userID, aiID sql.NullString
	switch id.PlayerType {
	case codenames.PlayerTypeHuman:
		userID.Valid = true
		userID.String = id.ID
	case codenames.PlayerTypeRobot:
		aiID.Valid = true
		aiID.String = id.ID
	default:
		return "", fmt.Errorf("unknown player type %q", id.PlayerType)
	}
	if _, err := tx.Exec(createPlayerStmt, pID, userID, aiID); err != nil {
		return "", fmt.Errorf("failed to insert player: %w", err)
	}
	return userID.String, nil
}

func (s *DB) Player(id codenames.PlayerID) (string, error) {
	var stmt string
	switch id.PlayerType {
	case codenames.PlayerTypeHuman:
		stmt = getUserPlayerStmt
	case codenames.PlayerTypeRobot:
		stmt = getAIPlayerStmt
	default:
		return "", fmt.Errorf("unknown player type %q", id.PlayerType)
	}
	var outID string
	if err := s.sdb.QueryRow(stmt, id.ID).Scan(&outID); err != nil {
		return "", err
	}
	return outID, nil
}

func (s *DB) BatchPlayerNames(pIDs []codenames.PlayerID) (map[codenames.PlayerID]string, error) {
	var userIDArgs, aiIDArgs []any
	for _, pID := range pIDs {
		switch pID.PlayerType {
		case codenames.PlayerTypeHuman:
			userIDArgs = append(userIDArgs, pID.ID)
		case codenames.PlayerTypeRobot:
			aiIDArgs = append(aiIDArgs, pID.ID)
		default:
			return nil, fmt.Errorf("unknown player type %q", pID.PlayerType)
		}
	}

	q := fmt.Sprintf(`
SELECT Users.display_name, Players.user_id, "user"
FROM Players
JOIN Users
  ON Users.id = Players.user_id
WHERE Users.id IN %s
UNION ALL
SELECT AIs.display_name, Players.ai_id, "ai"
FROM Players
JOIN AIs
  ON AIs.id = Players.ai_id
WHERE AIs.id IN %s`, groupedArgs(len(userIDArgs)), groupedArgs(len(aiIDArgs)))

	var allIDArgs = append(userIDArgs, aiIDArgs...)

	rows, err := s.sdb.Query(q, allIDArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query names: %w", err)
	}

	out := make(map[codenames.PlayerID]string)
	for rows.Next() {
		var name, id, typ string
		if err := rows.Scan(&name, &id, &typ); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		var playerType codenames.PlayerType
		switch typ {
		case "user":
			playerType = codenames.PlayerTypeHuman
		case "ai":
			playerType = codenames.PlayerTypeRobot
		default:
			return nil, fmt.Errorf("unexpected player type %q", typ)
		}
		pID := codenames.PlayerID{PlayerType: playerType, ID: id}
		out[pID] = name
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error scanning rows: %w", err)
	}

	return out, nil
}

func groupedArgs(n int) string {
	if n <= 0 {
		return "(NULL)"
	}
	return "(?" + strings.Repeat(",?", n-1) + ")"
}

func (s *DB) StartGame(gID codenames.GameID) error {
	if _, err := s.sdb.Exec(startGameStmt, gID); err != nil {
		return err
	}
	return nil
}

func (s *DB) UpdateState(gID codenames.GameID, gs *codenames.GameState) error {
	gsb, err := gameStateBytes(gs)
	if err != nil {
		return fmt.Errorf("failed to serialize game state: %w", err)
	}

	if _, err := s.sdb.Exec(updateGameStateStmt, gsb, gID); err != nil {
		return fmt.Errorf("failed to update game state: %w", err)
	}

	return nil
}

func (s *DB) uniqueID(tx *sql.Tx) (codenames.GameID, error) {
	i := 0
	var id codenames.GameID
	for {
		id = codenames.RandomGameID(s.r)
		var n int
		if err := tx.QueryRow(gameExistsStmt, id).Scan(&n); err != nil {
			return codenames.GameID(""), err
		}
		if n == 0 {
			break
		}
		i++
		if i >= 100 {
			return codenames.GameID(""), errors.New("tried 100 random IDs, all were taken, which seems fishy")
		}
	}
	return id, nil
}

func gameStateBytes(s *codenames.GameState) ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(&s)
	return buf.Bytes(), err
}

func gameStateFromBytes(dat []byte) (*codenames.GameState, error) {
	var gs codenames.GameState
	if err := gob.NewDecoder(bytes.NewReader(dat)).Decode(&gs); err != nil {
		return nil, fmt.Errorf("failed to load game state: %w", err)
	}
	return &gs, nil
}

# Codenames Bot Tournament

This branch is a stripped-down version of our Codenames project for the class
bot tournament. Students write a Codenames bot (a spymaster and an operative),
and matches are run **locally on one machine**. There is no hosting, and no
accounts beyond a name typed into the browser.

What's here:

- `cmd/codenames-server`: game server (REST + WebSocket, stores games in SQLite)
- `frontend/`: browser UI for setting up and watching a game
- `cmd/ai-server`: hosts the bots and joins them to games
- `game/`, `codenames/`: the rules engine and core types

## Install

You need **Go 1.24+**, **Node 20+**, and **pnpm** (`corepack enable` provides it).

```bash
git clone <repo-url> && cd ai_codenames
cd frontend && pnpm install && cd ..
go build ./...
```

## Where the bot interface is defined

A bot implements two Go interfaces from
[`codenames/codenames.go`](codenames/codenames.go):

```go
type Spymaster interface {
    // Sees the full board (all card colors) and returns a clue word + count.
    GiveClue(*Board, Agent) (*Clue, error)
}

type Operative interface {
    // Sees the board (colors of revealed cards only) and the clue,
    // and returns the codename to guess.
    Guess(*Board, *Clue) (string, error)
}
```

`Board`, `Card`, `Agent`, and `Clue` are defined in the same file. A tournament
bot is a single type implementing both (`AI` in
[`cmd/ai-server/server.go`](cmd/ai-server/server.go)). Optionally, it can also
implement `GuessOrPass(board, clue, mustGuess bool) (string, error)` and return
`codenames.PassGuess` to end its turn early.

### Registering a bot

Add it to the `backends` map in
[`cmd/ai-server/main.go`](cmd/ai-server/main.go):

```go
var backends = map[string]func() (AI, error){
    "team-alpha": func() (AI, error) { return alpha.New(), nil },
    "team-beta":  func() (AI, error) { return beta.New(), nil },
}
```

The map starts out **empty**, so the AI server won't start until at least
one bot is registered. Bots join games under their registered name.

## Running one match between two bots

Open three terminals in the repo root.

```bash
# 1. Game server (port 8080)
AUTH_SECRET=abc123 AI_SERVER_ENDPOINT=http://localhost:8081 go run ./cmd/codenames-server/

# 2. Bot server (port 8081)
AUTH_SECRET=abc123 WEB_SERVER_ENDPOINT=http://localhost:8080 go run ./cmd/ai-server/

# 3. Frontend (port 5173)
cd frontend && pnpm run dev
```

Then:

1. Open http://localhost:5173, enter any name, and create a game.
2. For each of the four seats, click **Add AI** and pick a bot from the dropdown
   (for example, `team-alpha` for both Red seats and `team-beta` for both Blue seats).
3. Start the game. The bots play automatically. The board and game log update live.

When a game finishes, its log is written as CSV to `logs/` (use `--log_dir` to change this).

### Quick sanity check without bots

`./cmd/codenames-local/run_local.sh` plays a game in the terminal, with all four
roles typed on stdin.

## Development

```bash
go build ./... && go vet ./... && go test ./...
cd frontend && pnpm run check
```

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

Students write bots as **standalone executables in any language**. For each
move, the engine starts the bot, writes one JSON request to its stdin, and reads
one JSON response from its stdout.

- [`BOT_GUIDE.md`](BOT_GUIDE.md): the guide for students (examples, legal-clue rules, timeouts, forfeits)
- [`botproto/schema.json`](botproto/schema.json): the formal JSON Schema
- [`botproto/botproto.go`](botproto/botproto.go): the same protocol as Go structs, plus clue validation
- [`botproto/match.go`](botproto/match.go): `RunMatch` plays one game between two students' bots
- [`examples/python/`](examples/python): a minimal spymaster and operative to use as a template

`go test ./botproto/` plays full games between the example bots. It needs `python3`.

The browser flow below uses the older in-process Go bot interface
(`Spymaster`/`Operative` in [`codenames/codenames.go`](codenames/codenames.go),
registered in [`cmd/ai-server/main.go`](cmd/ai-server/main.go)). It does not
run JSON bots yet.

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

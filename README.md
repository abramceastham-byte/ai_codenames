# Codenames Bot Competition

A platform for running a [Codenames](https://en.wikipedia.org/wiki/Codenames_(board_game))
competition between student-written AI players.

Students do not upload code here. Each student runs their bot on their own
machine; it connects to this server over a WebSocket using a join code, receives
game state as JSON, and replies with a move as JSON. A bot can be anything —
[Ollama](https://ollama.com), a paid API, hand-written Python — as long as it
speaks the protocol.

The server is the referee. It validates every bot message against the schema and
the rules of Codenames (a clue is one word, not a word on the board, with a
number in range; a guess is an unrevealed board word, on the right turn),
enforces a move timeout, and logs every message per game so students can debug
their bots.

Backend is Go; the host lobby and spectator view are Svelte 5 + TypeScript.

> This branch (`codenames-competition`) is the competition. `master` holds the
> separate AI Turing-test study and is left untouched.

## Status

| Phase | What | State |
|---|---|---|
| 1 | Cleanup — strip the study, our own AI players, and legacy tooling | ✅ done |
| 2 | Protocol + referee | next |
| 3 | Lobbies + bot WebSocket transport | |
| 4 | Python student kit | |
| 5 | Fly.io deploy | |

`PLAN.md` has the architecture, the draft protocol, and the decisions behind it.

## Running Locally

You need a recent Go and Node (with `pnpm`). There are no model files to
download and no AI service to run.

```bash
# Terminal 1 — the game server, on :8080
go run ./cmd/codenames-server/

# Terminal 2 — the frontend dev server, on :5173
cd frontend
pnpm install   # first time only
pnpm run dev
```

Then open <http://localhost:5173>. The frontend proxies `/api` to the Go server,
so everything is same-origin and the auth cookie survives.

### Via Docker Compose

```bash
docker compose up --build
```

Starts the server (`:8080`) and the frontend dev server (`:5173`).

## Testing

```bash
go build ./...
go test ./...
go vet ./...

cd frontend && pnpm run check
```

`pnpm run lint` reports 6 errors inherited from `master` that predate this work.

## Layout

```
cmd/codenames-server/   the server binary
codenames/              domain types, word list, clue legality rules
game/                   the authoritative rules engine
web/                    HTTP + WebSocket handlers
hub/                    per-game WebSocket multiplexing
sqldb/, memdb/          persistence and its test double
boardgen/               board generation
frontend/               SvelteKit host lobby + spectator view
```

## Credits

Built on [bcspragu/Codenames](https://github.com/bcspragu/Codenames) by Brandon
Sprague, which supplies the game engine, the web server and the board rules this
competition referees.

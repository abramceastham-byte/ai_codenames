# Agent Guide for the Codenames Bot Competition

A Codenames bot competition: students write AI players, run them on their own
machines, and connect them to this server over WebSocket to play each other.

**Branch rule:** work happens on `codenames-competition`. `master` holds the AI
Turing-test study — never commit to, merge into, rebase, or push it from here.

## 📂 Project Structure

- **Backend (Go)**
  - `cmd/codenames-server/`: the game server (API + bot WebSocket + static SPA).
  - `codenames/`: core domain types, the word list, and the clue legality check.
  - `game/`: the authoritative rules engine.
  - `web/`: HTTP handlers and WebSocket routing.
  - `hub/`: per-game WebSocket connection multiplexing.
  - `sqldb/` / `memdb/`: persistence and its in-memory test double.
  - `boardgen/`: board generation.

- **Frontend**
  - `frontend/`: SvelteKit (Svelte 5) host lobby and spectator view.

## 🛠 Development Workflow

### Backend (Go)

```bash
go build ./...
go test ./...
go vet ./...
```

### Frontend (Svelte)

From `frontend/`: `pnpm install`, `pnpm run dev`, `pnpm run check`,
`pnpm run lint`. Lint has 6 known pre-existing errors inherited from `master`.

## 🏗 Architecture & Patterns

- **API design**
  - REST for browser commands; WebSocket for live state
    (`/api/game/{id}/ws`).
  - A separate versioned JSON protocol for bots — see `PLAN.md` §4 and
    `web/README.md`.
  - Every move, from any source, is applied through `game.Move()`. That is the
    single rules path; do not add a second one.

- **Database**
  - SQLite (`sqldb`) for persistence, `memdb` for tests.

- **Referee**
  - The server validates bot messages and never trusts a client to enforce a
    rule. Clue legality lives in `codenames.ConflictingBoardWord`.

## 🚀 Deployment

A single Fly.io app, separate from the study, built from `Dockerfile.websrv`.
See `PLAN.md` §6 and (from Phase 5) `docs/DEPLOY.md`.

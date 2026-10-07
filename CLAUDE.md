# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Codenames **bot competition** platform: students write their own AI players,
run them on their own machines, and connect them to this server over WebSocket
to play against each other.

- **Go backend** — game server, referee, and bot protocol endpoint
- **Svelte 5 frontend** (SvelteKit, Tailwind CSS v4) — host lobby and spectator view
- **Python kit** (`kit/`, from Phase 4) — base class, examples, and offline runner for students

Branch note: this is `codenames-competition`. `master` holds the AI Turing-test
study and must keep all of its current functionality — never commit to, merge
into, rebase, or push `master` from this branch.

## Objective

Students build bots; bots play bots. A bot can be anything (Ollama, a paid API,
hand-written Python) as long as it speaks the protocol. The server is the
referee: it validates every bot message against the schema and the rules of
Codenames, enforces timeouts, and logs every message so students can debug.

See `PLAN.md` for the full build plan, the protocol draft, and the open
decisions.

## Dev Commands

### Backend (Go)

```bash
go build ./...          # Build all packages
go test ./...           # Run all tests
go test ./game/...      # Run tests for a specific package
go vet ./...            # Lint
```

### Frontend

```bash
cd frontend
pnpm install
pnpm run dev            # Dev server (port 5173)
pnpm run build          # Production build → build/
pnpm run check          # TypeScript type checking
pnpm run lint           # Prettier + ESLint
pnpm run format         # Auto-format
```

Note: `pnpm run lint` currently reports 6 pre-existing errors inherited from
`master` (`svelte/prefer-svelte-reactivity`, `no-explicit-any`, `prefer-const`,
`no-navigation-without-resolve`). They are unrelated to the competition work.

### Running Locally (2 terminals)

First-time setup: `cd frontend && pnpm install`.

```bash
# Terminal 1 — game server (port 8080)
go run ./cmd/codenames-server/

# Terminal 2 — frontend (port 5173)
cd frontend && pnpm run dev
```

Then open `http://localhost:5173`. There are no model files to download and no
AI service to start: every player is either a person in the browser or a student
bot connecting from outside.

### Running Locally (Docker Compose)

`docker compose up --build` starts the server and the frontend dev server.

## Architecture

### Backend Package Layout

**Domain:** `codenames/` defines core types (`Game`, `Board`, `Card`, `Agent`,
`Clue`) and the `DB` interface that all storage backends must satisfy. It also
holds the word list and `ConflictingBoardWord`, the morphological check that
rejects a clue sharing a root with a board word (`kings`, `kingdom`, `kingly`
are all illegal when `KING` is on the board) while allowing coincidental
containment (`bring` for `RING`). That function is the referee's clue rule.

**Game logic:** `game/` implements rules and turn management in two modes:
- `Play()` — fully automated game, used by referee tests
- `Move()` — single step, the one authoritative path for applying any move

Both use the `Spymaster` and `Operative` interfaces, so bots, people and test
fakes are interchangeable.

**Web service:** `web/` contains HTTP handlers and routing (REST + WebSocket).
REST handles browser commands (join, start, clue, guess); WebSocket broadcasts
state changes to everyone watching.

**Storage:** `sqldb/` implements `codenames.DB` with SQLite. `memdb/` provides an
in-memory implementation used in tests.

**Transport:** `hub/` manages WebSocket connection multiplexing per game.

**Messages:** `msgs/` defines the browser-facing WebSocket message types
(`RoleAssigned`, `GameStart`, `ClueGiven`, `GuessGiven`, `PlayerVote`,
`GameEnd`). The bot-facing protocol is separate and versioned — see `PLAN.md`
§4.

### Frontend

`src/lib/game.svelte.ts` is the central singleton `GameStore` that holds user
identity, the WebSocket connection, and all reactive game state (Svelte 5
runes).

`src/lib/api.ts` wraps REST calls. Components dispatch actions via REST; state
updates arrive via WebSocket and are applied to the store.

SvelteKit is configured as a fully static SPA (`adapter-static`,
`fallback: '200.html'`). `Dockerfile.websrv` builds it into the server image so
one URL serves the API, the bot socket and the UI.

### Key Environment Variables / Flags

The server accepts `--addr` (default `:8080`), `--db_path` (default
`codenames.db`), and `--hash_key_path`/`--block_key_path` for secure cookie keys
(auto-generated if missing). Every flag can also be set as an environment
variable via `peterbourgon/ff` (e.g. `DB_PATH`).

### Docker

```bash
docker build -t codenames-competition -f Dockerfile.websrv .
```

Multi-stage: Node builds the SPA, Go builds the server, both land in a
distroless image.

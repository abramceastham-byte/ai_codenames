# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Codenames is a full-stack implementation of the Codenames board game with:
- **Go backend** (game server + AI server)
- **Svelte 5 frontend** (SvelteKit, Tailwind CSS v4)
- An **AI server** that hosts tournament bots (registered in `cmd/ai-server/main.go`)

This branch is for a class bot tournament run locally. See README.md.

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

### Running Locally (3 terminals)

First-time setup: `cd frontend && pnpm install` (skip on subsequent runs).

```bash
# Terminal 1 — web server (port 8080)
AUTH_SECRET=abc123 AI_SERVER_ENDPOINT=http://localhost:8081 go run ./cmd/codenames-server/

# Terminal 2 — frontend (port 5173)
cd frontend && pnpm run dev

# Terminal 3 — AI server (port 8081, requires at least one registered bot)
AUTH_SECRET=abc123 WEB_SERVER_ENDPOINT=http://localhost:8080 go run ./cmd/ai-server/
```

Then open `http://localhost:5173`.

## Architecture

### Backend Package Layout

**Domain:** `codenames/` defines core types (`Game`, `Board`, `Card`, `Agent`, `Clue`) and the `DB` interface that all storage backends must satisfy.

**Game logic:** `game/` implements rules, turn management, and two execution modes:
- `Play()` — fully automated game (all players are bots)
- `Move()` — single step used by the web server for human/mixed games

Both modes use the `Spymaster` and `Operative` interfaces, making AI and human players interchangeable.

**Web service:** `web/` contains HTTP handlers and routing (REST + WebSocket). REST handles commands (join, start, clue, guess); WebSocket broadcasts state changes to all players. The `hub/` package manages WebSocket connection multiplexing.

**Storage:** `sqldb/` implements `codenames.DB` with SQLite. `memdb/` provides an in-memory implementation used in tests.

**AI:** `cmd/ai-server` hosts bots registered in its `backends` map; each implements `Spymaster` and `Operative`. `aiclient/` is the HTTP client the web server uses to call the AI server.

**Consensus:** `consensus/` tracks operative votes — a strict majority (>50%) is required to finalize a guess in multiplayer games.

**Messages:** `msgs/` defines all WebSocket message types with JSON marshaling (`RoleAssigned`, `GameStart`, `ClueGiven`, `GuessGiven`, `PlayerVote`, etc.).

### Frontend

`src/lib/game.svelte.ts` is the central singleton `GameStore` that holds user identity, the WebSocket connection, and all reactive game state (Svelte 5 runes).

`src/lib/api.ts` wraps REST calls. Components dispatch actions via REST; state updates arrive via WebSocket and are applied to the store.

SvelteKit is configured as a fully static SPA (`adapter-static`, `fallback: '200.html'`), so the built `frontend/build/` directory is served as static files by the Go web server.

### Key Environment Variables

| Variable | Used by | Purpose |
|---|---|---|
| `AUTH_SECRET` | both servers | Shared secret for inter-service auth |
| `AI_SERVER_ENDPOINT` | web server | URL of AI service |
| `WEB_SERVER_ENDPOINT` | AI server | URL of web service |
| `ENABLED_BACKENDS` | AI server | Comma-separated bots to load (default: all registered) |
| `DEFAULT_BACKEND` | AI server | Bot used when caller doesn't specify |

The web server also accepts `--addr` (default `:8080`), `--db_path` (default `codenames.db`), and `--hash_key_path`/`--block_key_path` for secure cookie keys (auto-generated if missing).

### Docker

```bash
docker build -t codenames-web -f Dockerfile.websrv .
docker build -t codenames-ai  -f Dockerfile.aisrv  .
```

Both use multi-stage builds (Go builder → scratch) for minimal images.

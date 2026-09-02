# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Codenames is a full-stack implementation of the Codenames board game with:
- **Go backend** (game server + AI server)
- **Svelte 5 frontend** (SvelteKit, Tailwind CSS v4)
- **Word2Vec-based AI** players using GloVe and ConceptNet models

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

# Terminal 3 — AI server (port 8081, LLM backend only; no model files needed)
AUTH_SECRET=abc123 \
WEB_SERVER_ENDPOINT=http://localhost:8080 \
ENABLED_BACKENDS=llm \
DEFAULT_BACKEND=llm \
OLLAMA_ENDPOINT=http://localhost:11434 \
OLLAMA_MODEL=qwq:32b \
go run ./cmd/ai-server/
```

Then open `http://localhost:5173`. The `llm` backend requires [Ollama](https://ollama.com) running with the model pulled (e.g. `ollama pull qwq:32b` — a ~20GB download); drop `llm` from `ENABLED_BACKENDS` if you only want the w2v player.

`qwq:32b` is a reasoning model: it "thinks" via inline `<think>...</think>` markup, which `llm/llm.go` pulls out before parsing the final answer and folds into the reasoning logged to `logs/ai_reasoning.jsonl` (visible via `/admin/{gameId}`) instead of discarding it. Expect much slower responses than a small instruct model like the old default (`llama3`) — `llm.DefaultTimeout` and the per-guess timeout budget in `llm/llm.go` are sized accordingly. qwq has no way to disable its `<think>` block, so it always pays this cost.

For faster responses, swap in a model that supports disabling reasoning (e.g. `qwen3:30b-a3b`) and pass `--ollama_think=false`:

```bash
# Terminal 3 — AI server, qwen3 with thinking disabled (faster, less deliberate)
AUTH_SECRET=abc123 \
WEB_SERVER_ENDPOINT=http://localhost:8080 \
ENABLED_BACKENDS=llm \
DEFAULT_BACKEND=llm \
OLLAMA_ENDPOINT=http://localhost:11434 \
OLLAMA_MODEL=qwen3:30b-a3b \
go run ./cmd/ai-server/ --ollama_think=false
```

`--ollama_think` (`true`/`false`, unset by default) sets Ollama's top-level `think` field, which forces reasoning on/off on models that support toggling it. Leaving it unset sends nothing and lets the model use its own default — the right choice for qwq, since it has no toggle. Other tunables: `--ollama_max_tokens` (num_predict budget, thinking + answer), `--ollama_timeout`, `--ollama_temperature`, `--ollama_seed` (fixes sampling for reproducible research runs).

### Running Locally (Docker Compose)

`docker compose up --build` starts all four services (a one-shot `model-setup` job, then web server, AI server, frontend) with sane dev defaults — see `docker-compose.yml` and the "Running Locally via Docker Compose" section in README.md. `model-setup` fetches + converts `data/glove.bin`/`data/conceptnet.bin` automatically (Dockerfile.modelsetup, using `scripts/setup_models.sh`), so no host-side Python is required for the Docker flow specifically — only the manual (non-Docker) flow needs Python installed to run that script directly.

## Architecture

### Backend Package Layout

**Domain:** `codenames/` defines core types (`Game`, `Board`, `Card`, `Agent`, `Clue`) and the `DB` interface that all storage backends must satisfy.

**Game logic:** `game/` implements rules, turn management, and two execution modes:
- `Play()` — fully automated game (all players are bots)
- `Move()` — single step used by the web server for human/mixed games

Both modes use the `Spymaster` and `Operative` interfaces, making AI and human players interchangeable.

**Web service:** `web/` contains HTTP handlers and routing (REST + WebSocket). REST handles commands (join, start, clue, guess); WebSocket broadcasts state changes to all players. The `hub/` package manages WebSocket connection multiplexing.

**Storage:** `sqldb/` implements `codenames.DB` with SQLite. `memdb/` provides an in-memory implementation used in tests.

**AI:** `w2v/` implements `Spymaster` and `Operative` using dual Word2Vec models (GloVe + ConceptNet). Scoring applies count bonuses and multi-word penalties. `aiclient/` is the HTTP client the web server uses to call the AI server.

**Consensus:** `consensus/` tracks operative votes — a strict majority (>50%) is required to finalize a guess in multiplayer games.

**Clue delay:** `cluedelay/` puts a floor (default 60s, `--clue_delay`) on how long a spymaster's clue phase lasts. A clue submitted sooner is held by the web server — not written to the DB, not broadcast — until the phase reaches that age, so an AI spymaster's response speed can't give it away. The spymaster's own client learns the release time from the clue response; nobody else sees anything until release.

The hold is per game and can be flipped mid-game by the game creator (`GET`/`POST /api/game/{id}/clueHold`, gated on `isGameCreator`), surfaced as a toggle in the setup and board views. Switching it off releases a clue that's mid-hold immediately. The setting is deliberately never broadcast and the endpoint refuses non-creators — an operative who knew the hold was off could read a three-second clue as a bot. State is in-memory, so a server restart returns every game to the `--clue_delay` default; `--clue_delay=0` disables holding server-wide and the per-game toggle then has no duration to switch on.

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
| `GLOVE_MODEL_PATH` | AI server | GloVe Word2Vec binary |
| `CONCEPT_NET_MODEL_PATH` | AI server | ConceptNet Word2Vec binary |
| `COMMON_WORDLIST` | AI server | Common words file for operative guessing |
| `ENABLED_BACKENDS` | AI server | Comma-separated backends to load (`w2v`, `llm`) |
| `DEFAULT_BACKEND` | AI server | Backend used when caller doesn't specify |
| `OLLAMA_ENDPOINT` | AI server | Ollama URL for the `llm` backend (default `http://localhost:11434`) |
| `OLLAMA_MODEL` | AI server | Ollama model name for the `llm` backend (default `qwq:32b`) |

The web server also accepts `--addr` (default `:8080`), `--db_path` (default `codenames.db`), `--clue_delay` (default `60s`, `0` to disable), and `--hash_key_path`/`--block_key_path` for secure cookie keys (auto-generated if missing).

### Docker

```bash
docker build -t codenames-web -f Dockerfile.websrv .
docker build -t codenames-ai  -f Dockerfile.aisrv  .
```

Both use multi-stage builds (Go builder → scratch) for minimal images.

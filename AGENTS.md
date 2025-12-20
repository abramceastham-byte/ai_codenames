# Agent Guide for Codenames

This repository contains a full-stack implementation of the Codenames board game, featuring a Go backend, multiple frontends (Next.js and Svelte), and an AI player based on Word2Vec.

## 📂 Project Structure

- **Backend (Go)**
  - `cmd/`: Entry points for services and tools.
    - `codenames-server/`: Main HTTP API server.
    - `ai-server/`: AI player service.
    - `codenames-local/`: CLI tool for playing locally.
  - `codenames/`: Core domain types and interfaces.
  - `web/`: HTTP handlers and WebSocket logic.
  - `game/`: Game logic implementation.
  - `sqldb/`: SQLite database implementation.
  - `vision/`: Cloud Vision API integration for board parsing.

- **Frontend**
  - `web/frontend/`: Next.js (v12) application (Legacy/Main).
  - `web/svelte/`: SvelteKit (Svelte 5) application (Newer/Experimental).

- **Infrastructure**
  - `Dockerfile`: Container definitions for various services.

## 🛠 Development Workflow

### Backend (Go)

- **Build Services**:
  ```bash
  go build ./cmd/codenames-server
  go build ./cmd/ai-server
  ```
- **Run Tests**:
  ```bash
  go test ./...
  ```
- **Run Locally**:
  The `codenames-local` tool allows playing without a server, but requires a Word2Vec model.
  ```bash
  go run ./cmd/codenames-local --model_file=/path/to/model.bin
  ```

### Frontend (Next.js)

Located in `web/frontend/`.

- **Install Dependencies**: `npm install`
- **Dev Server**: `npm run dev` (Runs on localhost:3000)
- **Build**: `npm run build`

### Frontend (Svelte)

Located in `web/svelte/`.

- **Install Dependencies**: `pnpm install`
- **Dev Server**: `pnpm run dev`
- **Check Types**: `pnpm run check`
- **Lint**: `pnpm run lint`

## 🏗 Architecture & Patterns

- **API Design**:
  - RESTful endpoints for CRUD actions (User, Game creation).
  - WebSockets for real-time game state updates (`/api/game/{id}/ws`).
  - See `web/README.md` for detailed API docs.

- **Database**:
  - Uses SQLite (`sqldb`) for persistence.
  - `memdb` available for in-memory testing.

- **AI Integration**:
  - Uses `word2vec` models for word associations.
  - Requires external model files (GoogleNews vectors or Project Gutenberg).
  - See root `README.md` for model acquisition instructions.

## 🚀 Deployment

- **Containerization**:
  - `cmd/codenames-server/Dockerfile`
  - `cmd/ai-server/Dockerfile`
  - `web/frontend/Dockerfile`
- **Production**: Deploys to `prod.bsprague.com` via Docker.

## ⚠️ Gotchas

- **Word2Vec Models**: AI features will crash or fail without a valid model file provided via flags.
- **Multiple Frontends**: Be aware of which frontend you are modifying (`web/frontend` vs `web/svelte`).
- **Go Version**: `go.mod` specifies a very new Go version (`1.25`), which might be a typo or experimental. Assume standard Go 1.20+ compatibility.

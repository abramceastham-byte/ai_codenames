# Agent Guide for Codenames

This repository contains a full-stack implementation of the Codenames board game, featuring a Go backend, a Svelte 5 static frontend, and an AI player based on Word2Vec.

## 📂 Project Structure

- **Backend (Go)**
  - `cmd/`: Entry points for services and tools.
    - `codenames-server/`: Main HTTP API server.
    - `ai-server/`: AI player service.
    - `codenames-local/`: CLI tool for playing locally.
  - `codenames/`: Core domain types and interfaces.
  - `embedding/`: Experimental embedding-based AI
  - `web/`: HTTP handlers and WebSocket logic.
  - `game/`: Game logic implementation.
  - `sqldb/`: SQLite database implementation.
  - `vision/`: Cloud Vision API integration for board parsing.

- **Frontend**
  - `frontend/`: SvelteKit (Svelte 5) application (Newer/Experimental).

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

### Frontend (Svelte)

Located in `frontend/`.

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
    - Stored in the `data/` directory
  - See `w2v/README.md` for model acquisition instructions.

## 🚀 Deployment

- **Containerization**:
  - `Dockerfile.websrv`
  - `Dockerfile.aisrv`
- **Production**: Deploys to `https://codenames.ai`, currently done manually

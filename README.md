# Codenames

Codenames is a hodgepodge of random components assembled over the course of several years, mostly related to playing the [Codenames](https://en.wikipedia.org/wiki/Codenames_(board_game)) board game.

The main draw of this repo is the backend server (`./cmd/codenames-server`) + web frontend (`./frontend`) for playing Codenames, which is live on https://codenames.ai. The "AI" part of the name comes from the fact that this repo also includes AI players (`./cmd/ai-server`) — both a [Word2Vec](https://en.wikipedia.org/wiki/Word2vec)-based algorithmic player and an LLM-backed player (via [Ollama](https://ollama.com)). Both can play in the same game, so you can pit one against the other.

This repo also includes scraps of code for:

- Playing a game of Codenames solo on your local computer against the backend server
  - This is in `./cmd/codenames-client`
- Parsing images of physical game boards using the [Cloud Vision API](https://cloud.google.com/vision/)
  - This never really worked well and is almost certainly a solved problem using a modern VLM.

Pretty much everything is written in Go, with the frontend in Svelte 5 + TypeScript

## Running Locally

Make sure you have a recent version of Go + TypeScript installed. If you want to use the LLM player, also install [Ollama](https://ollama.com) and pull a model (e.g. `ollama pull llama3`). Then:

```bash
# Run the backend web server
AUTH_SECRET=abc123 AI_SERVER_ENDPOINT=http://localhost:8081 go run ./cmd/codenames-server/

# In another terminal, run the frontend. (You can also use `npm` or tool of your choice)
cd frontend
pnpm install   # first time only
pnpm run dev

# In a third terminal, run the AI server (only needed for AI games).
# By default this loads BOTH the w2v and llm backends so you can pick which
# one each AI player uses from the lobby UI. Note that the model paths should
# be changed based on whatever model you download/train/use.
GLOVE_MODEL_PATH=data/glove.bin \
CONCEPT_NET_MODEL_PATH=data/conceptnet.bin \
COMMON_WORDLIST=data/common_words.txt \
AUTH_SECRET=abc123 \
WEB_SERVER_ENDPOINT=http://localhost:8080 \
ENABLED_BACKENDS=w2v,llm \
DEFAULT_BACKEND=w2v \
OLLAMA_ENDPOINT=http://localhost:11434 \
OLLAMA_MODEL=llama3 \
go run ./cmd/ai-server/
```

From here, you can open a web browser to `http://localhost:5173` to start a game.

## AI Backends

The AI server supports two backends, configured via `ENABLED_BACKENDS` (comma-separated):

| Backend | Description | Required env vars |
|---|---|---|
| `w2v` | Algorithmic player using GloVe + ConceptNet word embeddings | `GLOVE_MODEL_PATH`, `CONCEPT_NET_MODEL_PATH`, `COMMON_WORDLIST` |
| `llm` | Large language model player via Ollama | `OLLAMA_ENDPOINT` (default `http://localhost:11434`), `OLLAMA_MODEL` (default `llama3`) |

`DEFAULT_BACKEND` selects which one is used when a caller doesn't specify (defaults to the first enabled backend, alphabetically). You can also enable just one — e.g. `ENABLED_BACKENDS=llm` skips loading the w2v models entirely.

### Picking a backend per AI player

In the game lobby, when the game creator clicks **Add AI** for a role, a small dropdown next to the button lets them pick which backend to use (`default`, `w2v`, or `llm`). This means you can stage matchups like w2v spymaster vs LLM spymaster on the same board.

The available backends are exposed at `GET /api/ai/backends`, and the per-AI choice is sent as a `backend` field on `POST /api/game/{id}/requestAI`.

### Spectating

If you create a game without joining a role yourself, you'll spectate as a non-spymaster — useful for watching AI vs AI matches.

### LLM prompts

Prompts for clue-giving and guessing live in `./llm/llm.go`. The model gets the team's words, the opponent's words, bystanders, and the assassin, and is asked to respond in `WORD COUNT` format. Operative responses are validated against the board with up to 3 retries before falling back to a random unrevealed card.

## Word2Vec Models

See the docs in `./w2v/README.md` for more details on the Word2Vec setup.

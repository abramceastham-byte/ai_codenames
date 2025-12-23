# Codenames

Codenames is a hodgepodge of random components assembled over the course of several years, mostly related to playing the [Codenames](https://en.wikipedia.org/wiki/Codenames_(board_game)) board game.

The main draw of this repo is the backend server (`./cmd/codenames-server`) + web frontend (`./frontend`) for playing Codenames, which is live on https://codenames.ai. The "AI" part of the name comes from the fact that this repo also includes a [Word2Vec](https://en.wikipedia.org/wiki/Word2vec)-based AI player (`./cmd/ai-server`)

This repo also includes scraps of code for:

- Playing a game of Codenames solo on your local computer against the backend server
  - This is in `./cmd/codenames-client`
- Parsing images of physical game boards using the [Cloud Vision API](https://cloud.google.com/vision/)
  - This never really worked well and is almost certainly a solved problem using a modern VLM.

Pretty much everything is written in Go, with the frontend in Svelte 5 + TypeScript

## Running Locally

Make sure you have a recent version of Go + TypeScript installed, then run:

```bash
# Run the backend web server
AUTH_SECRET=abc123 AI_SERVER_ENDPOINT=http://localhost:8081 go run ./cmd/codenames-server/

# In another terminal, run the frontend. (You can also use `npm` or tool of your choice)
cd frontend
pnpm run dev

# In a third terminal, run the AI server (only needed for AI games). Note that the models path should be changed based on whatever model you download/train/use.
GLOVE_MODEL_PATH=data/glove.bin \
CONCEPT_NET_MODEL_PATH=data/conceptnet.bin \
AUTH_SECRET=abc123 \
WEB_SERVER_ENDPOINT=http://localhost:8080 \
go run ./cmd/ai-server/
```

From here, you can open a web browser to `http://localhost:5173` to start a game.

## Word2Vec Models

All the AI stuff is based on Word2Vec, see the docs in `./w2v/README.md` for more details.

# Single-machine deployment

This stack runs one web process, one AI process, and Caddy. That topology is intentional: game fan-out and consensus state are in memory, and SQLite is the persistent store.

## Bring-up

1. Install Docker Engine and the Compose plugin. Point the domain's A/AAAA records at the machine and allow inbound TCP 80 and 443.
2. Copy `.env.example` to `.env`, set the domain, and replace `AUTH_SECRET` with output from `openssl rand -hex 32`.
3. For Word2Vec, place `glove.bin`, `conceptnet.bin`, and `common_words.txt` in `data/` and set `ENABLED_BACKENDS=w2v` (or `w2v,llm`).
4. For bundled Ollama, run `docker compose --profile ollama up -d ollama`, then `docker compose exec ollama ollama pull "$OLLAMA_MODEL"`.
5. Run `docker compose --profile ollama up -d --build` for bundled Ollama, or `docker compose up -d --build` for Word2Vec or an external Ollama endpoint.
6. Check `docker compose ps`, `docker compose logs web ai`, and `https://$DOMAIN/healthz`.

The `app_state` volume contains the SQLite database, cookie keys, and research CSV logs. Back it up off-machine and test restoration before recruiting participants.

## Study launch gate

Do not treat the current browser-generated CSV as authoritative research data. The browser derives model labels from display names (which are randomized) and submits event rows itself. Before collecting study data, generate immutable event and condition records on the server, store the selected AI backend against the robot ID, assign participant/session IDs, record consent/version metadata, and export only trusted server-side records.

Freeze the Ollama model name and digest, prompt version, sampling settings, hardware, and software commit for every run. Benchmark response latency under expected concurrency; latency is a likely condition-revealing confound.

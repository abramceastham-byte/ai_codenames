# Launch Plan: AI Codenames Turing Test — Going Live

**Goal:** Deploy the game publicly so real users can join games, play against each other
and AI players, and run Turing-test games (guess which players are AI).

**Audience:** This plan is written to be executed by Claude (Opus) working in this repo.
Each phase has concrete tasks and acceptance criteria. Phases must be done in order;
tasks within a phase are mostly independent.

**Recommended target:** a single Linux VPS (8 GB RAM minimum; 16 GB if running Ollama)
running Docker Compose behind Caddy for automatic HTTPS. The app cannot horizontally
scale anyway (SQLite + in-process WebSocket hub + in-process consensus state), so one
sticky instance is the honest architecture. Cost ~$10–40/mo depending on LLM choice.

---

## Phase 0 — Decisions needed from the user (ask before starting)

1. **Domain name** — what hostname will the game live at? (e.g. `codenames.example.com`).
   Needed for TLS, cookie settings, and the frontend build (`PUBLIC_API_URL` is baked in
   at build time).
2. **Hosting provider** — any VPS works (Hetzner, DigitalOcean, Lightsail…). Needs
   Docker, ports 80/443, and ~5 GB disk for model files + images.
3. **LLM backend at launch** — pick one:
   - **w2v only** (drop `llm` from `ENABLED_BACKENDS`): zero extra cost, runs on an
     8 GB box, but the w2v player is the more "mechanical" one.
   - **Ollama on the VPS**: llama3-8B on CPU needs ~6 GB RAM and can take 30–60 s per
     clue — this can exceed the 8–25 s human-like delay window
     (`cmd/ai-server/server.go`) and become a tell. Only acceptable on a strong CPU box;
     test latency before committing.
   - **GPU instance for Ollama**: best AI quality + latency, highest cost.
4. **Who is "admin"?** The UPDATES.txt to-do list says game logs should be visible to
   admins only. Decide how admin is identified (simplest: an `ADMIN_SECRET` env var +
   query param/header) before Phase 2 task 2.6.

---

## Phase 1 — Land the Turing Test mode on master

The Turing mode (simultaneous clues, neutral operatives, 60 s reveal, AI/human vote —
see memory/project notes) lives on `feature/two-guesses` and is **not merged**. Its
last commit is `10f7bad "Attempting to fix start button bug"`, i.e. it may be unfinished.

1.1. Rebase `feature/two-guesses` onto current `master` (master has since gained random
     usernames, AI pass support, and thinking delays — expect conflicts in
     `cmd/ai-server/server.go`, `web/web.go`, `frontend/src/lib/game.svelte.ts`).

1.2. Reproduce and fix the start-button bug the last commit was attempting to fix.
     Run the full local stack (3 terminals per CLAUDE.md), create a Turing game,
     and exercise the start flow with mixed human/AI rosters.

1.3. Play at least 2 full Turing-mode games locally end-to-end (clue → guesses →
     reveal → vote → scoring), plus 1 classic game to confirm no regression.

1.4. `go test ./...`, `go vet ./...`, `cd frontend && pnpm run check && pnpm run lint`.

1.5. Merge to master.

**Acceptance:** Turing-mode and classic games both complete cleanly on master; tests pass.

---

## Phase 2 — Production-readiness code changes

These are all on master, each its own commit.

2.1. **Serve the frontend from the Go server.** Despite what CLAUDE.md says,
     `web/web.go:initMux` only registers `/api/...` routes — there is no static file
     serving. Add it: serve a directory (new `--static_dir` flag in
     `cmd/codenames-server/main.go`) with an SPA fallback to `200.html` for any
     non-`/api` path. This makes frontend and API same-origin, which simplifies
     cookies, CORS, and WebSockets. Update CLAUDE.md to match reality.

2.2. **Lock down CORS.** `cmd/codenames-server/main.go:82` currently allows **every**
     origin with credentials. Replace with an `--allowed_origin` flag (the public URL);
     with same-origin serving from 2.1 CORS is only needed for local dev, so default
     it to `http://localhost:5173`.

2.3. **Secure cookies.** `web/web.go:275` sets `Secure: false`. Make it configurable
     (`--secure_cookies`, default true in prod) and keep `SameSite=Lax`.

2.4. **WebSocket origin check.** `web/web.go:55` `CheckOrigin` allows everything.
     Accept only the configured origin (and localhost in dev).

2.5. **Fix the Dockerfiles — both currently fail to build:**
     - `Dockerfile.websrv` is missing `COPY names/ names/` (web server imports it)
       and, after 2.1, needs a stage that builds the frontend (`node:22` + pnpm →
       copy `frontend/build/` into the final image) since `scratch` images can't be
       populated at runtime. `PUBLIC_API_URL` must be passed as a build arg.
     - `Dockerfile.aisrv` is missing `COPY llm/ llm/` (ai-server imports it).
     - Delete or update the stray third Dockerfile at `cmd/codenames-server/Dockerfile`.
     - Verify with `docker build` for both images.

2.6. **Admin-gated game logs** (open UPDATES.txt to-do): the per-game log/history
     view must not be visible to players mid-game (it would reveal who's AI). Gate it
     behind the admin mechanism chosen in Phase 0.4, and ensure the log records player
     type (human/AI) and AI backend (W2V vs LLM) — the old name-prefix detection broke
     when usernames went random; expose player type from the backend instead.

2.7. **Health endpoints.** Add `GET /healthz` to both servers (web: also ping DB;
     AI: confirm models loaded) for Compose healthchecks and uptime monitoring.

2.8. **Secrets hygiene.** `AUTH_SECRET=abc123` must not reach prod — generate a long
     random secret. Confirm `hashKey`/`blockKey` (secure-cookie keys, currently sitting
     in the repo root) and `codenames.db*` are gitignored; generate fresh keys on the
     server, never reuse the dev ones.

**Acceptance:** `docker build` succeeds for both images; running the web container
alone serves the SPA and API on one port; cookies are Secure; cross-origin requests
from a random origin are rejected.

---

## Phase 3 — Deployment artifacts

3.1. **`docker-compose.yml`** at repo root with services:
     - `caddy` (ports 80/443, automatic Let's Encrypt TLS, reverse-proxy to `websrv:8080`
       — Caddy proxies WebSockets transparently, no extra config).
     - `websrv` — env: `AUTH_SECRET`, `AI_SERVER_ENDPOINT=http://aisrv:8081`; volumes
       for the SQLite db file, cookie keys, and `logs/`.
     - `aisrv` — env per the CLAUDE.md table; volume mounting `data/` (models are
       ~1.1 GB — mount, don't bake into the image); `ENABLED_BACKENDS` per Phase 0
       decision.
     - `ollama` — **only if** the LLM backend was chosen; volume for model storage;
       document `docker compose exec ollama ollama pull llama3` as a one-time step.
     - `restart: unless-stopped` and healthchecks (from 2.7) on everything.

3.2. **`Caddyfile`** — single site block for the domain, `reverse_proxy websrv:8080`.

3.3. **`.env.example`** documenting every variable, and a `deploy/README.md` with the
     exact bring-up steps (see Phase 4).

3.4. Verify the whole stack **locally** with `docker compose up` (use a self-signed /
     localhost Caddy config or bypass caddy) and play one full game through it.

**Acceptance:** fresh `docker compose up -d` on a clean machine + model files = playable game.

---

## Phase 4 — Server bring-up (run on the VPS)

4.1. Provision the VPS (Phase 0.2), point the domain's A/AAAA record at it.
4.2. Install Docker + Compose plugin; enable a basic firewall (allow 22/80/443 only).
4.3. Clone the repo; copy the two model binaries + wordlist into `data/`
     (`glove.bin` 462 MB, `conceptnet.bin` 598 MB, `common_words.txt` — scp them;
     do **not** commit them).
4.4. Create `.env` from `.env.example` with real secrets (`openssl rand -hex 32`).
4.5. `docker compose up -d`; if using Ollama, pull the model.
4.6. Smoke test over the public URL: create game, join from a phone (different
     network), add AI players, play a Turing round, confirm WebSocket updates arrive
     and cookies persist across refresh.

**Acceptance:** a stranger with just the URL can join and finish a Turing game.

---

## Phase 5 — Operations for the experiment

5.1. **Back up the data you're running this for.** Nightly cron on the VPS:
     `sqlite3 codenames.db ".backup ..."` + tar of `logs/` (the Turing-test results —
     `logs/all_games.csv` — are the point of the project), synced off-box
     (rclone to object storage or even a private git repo for the CSVs).
5.2. **Uptime monitoring** on `/healthz` (free tier of UptimeRobot or similar).
5.3. **Capacity note:** AI calls block on model inference; the w2v models hold
     ~1 GB resident. One 8 GB box comfortably handles tens of concurrent games with
     w2v; with CPU Ollama, concurrent LLM games queue behind each other — cap
     concurrent LLM players or note it as a known limit.
5.4. **Update path:** `git pull && docker compose build && docker compose up -d`
     (brief downtime is fine for this stage; in-flight games live in SQLite and
     survive restarts only if game state is fully persisted — verify, and if active
     games don't survive a restart, deploy during quiet hours).

---

## Phase 6 — Pre-launch checklist

- [ ] Turing mode merged and playable on master (Phase 1)
- [ ] Both Docker images build and run (2.5)
- [ ] CORS/cookies/WS origin locked to the prod domain (2.2–2.4)
- [ ] Game log hidden from players, shows W2V/LLM for admins (2.6)
- [ ] Real `AUTH_SECRET` + fresh cookie keys on the server (2.8)
- [ ] HTTPS works, HTTP redirects (Phase 3/4)
- [ ] Full game completed from two devices on different networks (4.6)
- [ ] AI response latency stays inside the 8–25 s human-like window (Phase 0.3)
- [ ] Backups running and restorable (5.1)
- [ ] Open UPDATES.txt to-dos either done or consciously deferred

## Out of scope for launch (deliberately)

- Horizontal scaling / Postgres migration — SQLite on one box is fine for this stage.
- Accounts/passwords — anonymous random-username join is part of the experiment design.
- A hosted-LLM-API backend (Claude/OpenAI) as an alternative to Ollama — good future
  option if CPU latency is a problem, but it's new code, not deployment work.

# PLAN.md — Codenames Bot Competition

Branch: `codenames-competition` (cut from `master`). Nothing in this plan touches
`master`; all work lands here. Approved 2026-10-07; Phase 1 is complete.

## Decisions locked in (2026-10-07)

| # | Decision |
|---|---|
| Q1 | **Two students per match** — one bot per team, playing spymaster *and* operative. Multi-seat `hello` keeps a 4-student format one flag away; to be checked with Mourning. |
| Q2 | **Keep the frontend** as host lobby + spectator board + bot-log viewer, so people can watch the bots compete. |
| Q5 | **Study logs are backed up** to a separate folder; `logs/` is deleted in Phase 1. |
| Q8 | **No existing deployment constrains anything** — build the tournament Fly app fresh. |
| Q9 | **Prompt tuning preserved** as commit `8714b66` on branch `study/llm-prompt-tuning` (off `master`, not merged into it). |

Settled on the recommendations in §8, no further input needed:

| # | Decision |
|---|---|
| Q3 | Clue counts restricted to `1 … (your unrevealed words)`; the official "zero"/"unlimited" clue is dropped. |
| Q4 | Move timeout defaults to **60 s**, not 30 — a student's local Ollama needs the room — and is settable per game. |
| Q6 | Human seats stay playable in the browser, so a student can play against their own bot. |
| Q7 | Tournament scheduling, leaderboard and Elo are **out of scope for v1**; the `match_result` log record is shaped so a leaderboard can be built on it later. |
| Q10 | `pip install -e kit/` from a clone, with PyPI as a later drop-in. |
| Q11 | Join code plus a self-declared `bot_name`; the protocol reserves an optional `auth_token` in `hello` so per-student tokens need no v2. |
| Q12 | **No history rewrite.** New commits carry no attribution trailer; the 13 existing ones are left alone. |

---

## 1. Current architecture

Three deployed pieces plus a pile of research tooling.

### Backend (Go, module `github.com/bcspragu/Codenames`)

**Domain — `codenames/`.** `Game`, `GameState`, `Board`, `Card`, `Clue`, `Agent`,
`Team`, `Role`, `PlayerID`, and the `DB` interface. Also `words.go` (the 400-word
Codenames deck) and `ConflictingBoardWord` / `SuspectedCompound` — a careful,
well-tested morphological check that rejects a clue sharing a root with a board
word (`kings`, `kingdom`, `kingly` all illegal when `KING` is on the board) while
allowing coincidental containment (`bring` for `RING`). `RandomGameID` already
produces pronounceable three-word IDs like `SmugglerCourtUnicorn`.

**Rules — `game/`.** The authoritative engine. `Move()` applies one action
(`GIVE_CLUE` / `GUESS`) to a state and returns the new state plus status; it
enforces turn order, the board-word clue ban, the `count + 1` bonus guess, turn
end on a wrong guess, and win/loss. `Play()` runs a fully automated game through
the `Spymaster` / `Operative` interfaces. `codenames.GameOver` handles the
9/8-card asymmetry and the assassin.

**Web — `web/web.go` (1,708 lines).** `gorilla/mux` REST for commands
(`/api/user`, `/api/game`, `.../join`, `.../assignRole`, `.../start`,
`.../clue`, `.../guess`), auth via a `securecookie`-encrypted `Authorization`
cookie, and a `requireGameAuth` middleware that loads the game + roster and
gates on creator / role / status. A `GET /api/game/{id}/ws` WebSocket is
**broadcast-only** — `hub/conn.go`'s `readPump` reads frames purely to keep the
socket alive and **discards every inbound message**. `broadcastMessage` sends
the full-colour board to spymasters and a `codenames.Revealed`-masked board to
everyone else.

**Transport — `hub/`.** Per-game connection registry with `ToGame`, `ToPlayer`,
`Disconnect`. One goroutine pair per connection. No inbound routing.

**Messages — `msgs/`.** `RoleAssigned`, `GameStart`, `ClueGiven`, `PlayerVote`,
`GuessGiven`, `GameEnd`, each with a custom `MarshalJSON` that stamps an
`"action"` discriminator. Server→client only; unversioned.

**Storage — `sqldb/` (SQLite via `modernc.org/sqlite`) and `memdb/`** behind
`codenames.DB`. Tables: `Users`, `AIs`, `Players`, `Games`, `GamePlayers`,
`GameHistory`.

**AI — `cmd/ai-server/` (934 lines) + `w2v/`, `llm/`, `embedding/`,
`operative/`.** A second HTTP service. `POST /join` makes it create a robot user
on the web server via `client/` (full cookie-jar HTTP client), join, claim a
role, open the broadcast WebSocket, and then drive moves back through REST. Two
backends: `w2v` (dual GloVe + ConceptNet word2vec) and `llm` (Ollama, 1,202
lines of prompt engineering in `llm/llm.go`). Writes `logs/ai_reasoning.jsonl`.

**Study instrumentation.** This is a Turing-test platform, and it shows:

- `cluedelay/` — holds a spymaster's clue for 60 s from the *start* of the clue
  phase so an AI's response speed isn't a tell. Per-game toggle, creator-only,
  deliberately never broadcast.
- `names/` — every player, human or AI, gets a random `SilentOtter42` name so
  names can't identify AIs.
- `sanitizePlayerID` — strips `HUMAN`/`ROBOT` from every player ID on the wire.
- `consensus/` — strict-majority voting among multiple operatives per team.
- `web.serveGameLog` → `logs/all_games.csv`; `serveAdminGameLog` +
  `/admin/{gameId}` → the reasoning log; `scripts/merge_reasoning_data.py`.
- `cmd/batchtest/` — offline LLM-vs-LLM batch harness for prompt tuning.

**Research/legacy tooling.** `vision/` + `dict/` (Google Cloud Vision board
photo parsing), `cmd/w2v-topn`, `cmd/w2v-filter`, `cmd/word-server`,
`cmd/boardgen-cli`, `cmd/codenames-local`, `cmd/codenames-client`, `io/`,
`embeddings/` (a separate Python FastAPI embedding service),
`scripts/convert_to_w2v_binary.py`, `scripts/setup_models.sh`,
`Dockerfile.modelsetup`.

### Frontend (`frontend/`, SvelteKit + Svelte 5 runes + Tailwind 4)

Static SPA (`adapter-static`, `fallback: 200.html`). `src/lib/game.svelte.ts` is
a singleton `GameStore` holding identity, the WebSocket, and reactive state.
REST out via `api.ts`, state in via WebSocket. Views: `SetupView` (lobby),
`BoardView`, `GameControls`, `GameLog`, `TurnHistory`, `ClueHoldToggle`, plus an
`/admin/[gameId]` reasoning viewer.

### Two things I found that contradict the docs

1. **The Go server does not serve the frontend.** `CLAUDE.md` says the built
   `frontend/build/` is served by the Go web server, but `web/initMux` registers
   only `/api/...` routes and there is no `http.FileServer` anywhere in the repo.
   `frontend/build_and_upload.sh` `kubectl cp`s a tarball into a Caddy PVC — the
   SPA is deployed separately today. Serving one URL on Fly.io therefore needs
   static-file serving *added*; it is not a config change.
2. **There is no Fly.io config in the repo.** No `fly.toml`, no `flyctl`
   reference in any file, nothing in git history. So "a separate Fly app" has
   nothing to collide with, but also no existing setup to copy. I need to know
   how the study is actually deployed (see Q8).

### Test state before any changes

`go test ./...` has **one pre-existing failure**:
`llm.TestDecideGuess/optional_guess_above_mandated_threshold_guesses` (expects
`"Pot"`, gets `"<pass>"`). It fails identically at `HEAD` with your uncommitted
`llm/llm.go` stashed, so it is not caused by the in-progress edit. The `llm`
package is on the delete list, so Phase 1 makes this failure go away rather than
fixing it. Everything else passes.

Also: your working tree has uncommitted changes to `llm/llm.go` (+89/-6, prompt
tuning). Deleting that package discards them. I will not touch it until you say
so — see Q9.

---

## 2. Recommended approach for how bots plug in

**Your design is right, and I recommend it.** Students running their own process
that dials out over WebSocket is the correct architecture here, and I want to be
explicit about why, because the alternatives are worse in ways that matter:

- *Uploading code to our server* means sandboxing untrusted student code,
  CPU/memory/wall-clock limits, a dependency story per language, and a secrets
  problem the moment someone wants to use a paid API key. It also locks everyone
  into whatever runtime we provision.
- *Outbound WebSocket from the student's machine* needs no inbound port, no
  firewall rules, and no NAT traversal on their side. It works from a dorm
  network, a laptop on wifi, and a lab machine identically. Their API keys and
  their Ollama instance stay on their hardware.
- *Language-agnostic by construction*: the contract is JSON over a socket, so a
  hand-written Python script, a Node bot, or a Go bot are all first-class. The
  Python kit is a convenience, not the interface.

Three refinements I do want to argue for, as departures from how the current
code works:

**(a) One WebSocket for everything, no cookies, no REST for bots.** The existing
bot path (`cmd/ai-server` → `client/`) creates a user over REST, carries a
`SameSite=Lax` encrypted cookie in a cookie jar, joins, assigns a role, opens a
read-only socket, and then POSTs moves back. That is five HTTP round-trips of
auth ceremony that a student would have to reimplement, and the cookie is
exactly the part that breaks silently in a browser-less client. Instead: the bot
opens one socket to `/bot/v1/ws`, its first frame is a `hello` carrying the join
code, and everything — lobby, state, requests, moves — flows over that one
connection. A disconnect is unambiguous: the bot is gone. This is a
*replacement* for the bot transport, not an addition; the browser keeps its
cookie+REST path.

**(b) Strict request/response, one outstanding request per seat.** (With the
per-team format now settled, a single connection holds both of a team's seats
and answers clue requests and guess requests on the same socket.) The server
sends `clue_request` / `guess_request` with a `request_id` and waits; the bot
replies with that same `request_id`. The server never has two open requests for
one seat. This makes a student's bot a pure function of the state it was handed,
makes the referee's timeout logic a single timer, and makes `consensus/`
unnecessary (one guesser per seat, so there is nothing to vote on). Event
messages (`clue_given`, `guess_given`, `turn_end`) are informational and need no
reply, so a bot that ignores them still plays legally.

**(c) A protocol DTO layer, not reuse of `codenames` types on the wire.** The
current JSON is hostile to a student: `Card.Codename` serialises as `"codeword"`,
and `Agent` is a **bare integer** (`1` = red, `4` = assassin). Bots get a
hand-written v1 DTO with `"word"` and string enums (`"RED"`, `"ASSASSIN"`). This
also decouples the wire format from internal refactors, which is the whole point
of versioning it.

---

## 3. Delete list

Shown first, as asked. Nothing here is referenced by anything I plan to keep — I
checked every import. Total: ~30 MB of tracked binaries and ~7,000 lines of Go.

### 3a. Our own AI players, models and prompt pipeline

| Path | Why |
|---|---|
| `llm/` (`llm.go`, `summary.go`, both tests) | Our Ollama player. 1,202 lines of prompts. Students bring their own. |
| `w2v/` | GloVe + ConceptNet word2vec player. |
| `embedding/` | Experimental remote-embedding player. Unreferenced already. |
| `embeddings/` | Separate Python FastAPI embedding service (+ `uv.lock`, `__pycache__`). |
| `operative/` (+ test) | `AllowBonus` — our bot's bonus-guess policy. A student's own decision now. |
| `cmd/ai-server/` (`main.go`, `server.go`, `Dockerfile`) | The entire AI service. Replaced by student-run bots. |
| `aiclient/` | Web server → AI server client. |
| `client/` (`client.go`, `ws.go`) | Go REST+WS client, only used by the AI server and the CLI client. |
| `cmd/batchtest/` | Offline LLM-vs-LLM prompt-tuning harness. |
| `cmd/w2v-topn/` (+ `combinations.go`, `official.txt`) | word2vec exploration tool. |
| `cmd/w2v-filter/` | One-off wordlist filter against w2v corpora. |
| `cmd/word-server/` | Standalone word2vec query server. |
| `scripts/convert_to_w2v_binary.py`, `scripts/setup_models.sh` | Model download/convert pipeline. |
| `Dockerfile.modelsetup`, `Dockerfile.aisrv` | Model-setup and AI-server images. |
| `data/common_words.txt`, `data/common_words_filtered.txt` | w2v guessing wordlists (tracked, 391 KB). |

### 3b. Study-only and data-collection code

| Path | Why |
|---|---|
| `cluedelay/` (+ test, 445 lines) | 60-second clue hold so an AI's speed isn't a tell. A Turing-test device; in a bot competition everyone is a bot. Replaced by a *move timeout*, which is a different mechanism (see §5). |
| `names/` | Random anonymous names so AIs can't be identified. Competition bots should show the name the student chose. |
| `consensus/` | Strict-majority voting among multiple operatives. One guesser per seat now. |
| `web.serveGameLog` + `/api/game/{id}/log` | Writes `logs/all_games.csv` research CSV. |
| `web.serveAdminGameLog` + `requireAdmin` + `adminPlayer` + `/api/admin/game/{id}/log` | AI-reasoning viewer and the `ADMIN_SECRET` trust model. |
| `web.sanitizePlayerID` | Hides which players are AI. Counterproductive here. |
| `web.serveClueHold` / `serveSetClueHold` + `/api/game/{id}/clueHold` (GET+POST) | Per-game clue-hold toggle. |
| `web.serveCreateAI`, `serveAIBackends`, `serveRequestAI` + `/api/ai`, `/api/ai/backends`, `/api/game/{id}/requestAI` | Asking our AI server to seat a player. |
| `scripts/merge_reasoning_data.py` | Correlates reasoning JSONL with raw model logs for research review. |
| `logs/ai_reasoning.jsonl`, `logs/all_games.csv`, `logs/game-SmugglerCourtUnicorn.csv` | Collected study data. Only the last is tracked; see Q5 before I delete any of it. |
| `frontend/src/lib/components/ClueHoldToggle.svelte` | UI for the clue hold. |
| `frontend/src/routes/admin/[gameId]/` | Reasoning-log viewer. |
| `docs/LAUNCH_PLAN.md` | Launch plan for the Turing-test study. |

### 3c. Unrelated legacy

| Path | Why |
|---|---|
| `vision/` (+ test) | Google Cloud Vision board-photo parsing. Unreferenced. Drops 5 heavy `cloud.google.com/go` + `genproto` deps. |
| `dict/` | Word lookup, only used by `vision/`. |
| `io/` | Terminal `Spymaster`/`Operative` ("No idea what this is" — its own doc comment). |
| `cmd/codenames-local/` (+ `README.md`, `run_ai.sh`, `run_local.sh`) | CLI local game using `io/` + `llm` + `w2v`. |
| `cmd/codenames-client/` | CLI client using `client/`. |
| `cmd/boardgen-cli/` | Prints a random board. |

### 3d. Tracked build artifacts and junk (~30 MB)

`ai-server` (10.9 MB), `batchtest` (9.3 MB), `codenames-local` (9.8 MB),
`bin/ai-server`, `bin/codenames-server`, `cmd/boardgen-cli/boardgen-cli`,
`embeddings/__pycache__/`. Plus untracked local state: `codenames.db*`,
`hashKey`, `blockKey` (already gitignored; I will leave your local copies alone
and only add ignore rules).

### 3e. Deliberately **not** deleting

- `codenames/` — `ConflictingBoardWord` is exactly the referee check that
  requirement 3 asks for, already written and tested. Keeping it.
- `game/` — the rules engine is the referee's core. Keeping all of it; `Play()`
  stays because it is how the Go-side referee tests play full games.
- `frontend/` — see Q2. My recommendation is to keep it as a host + spectator
  view. A bot competition where nobody can watch the games is much worse, and
  "watch my bot make a dumb move" is the single best debugging tool a student
  has.
- `UPDATES.txt`, `docs/original_design.md`, `docs/whiteboard_design.jpg` — repo
  history, harmless. See Q5.
- `cmd/codenames-server/`, `web/`, `hub/`, `msgs/`, `sqldb/`, `memdb/`,
  `boardgen/`, `httperr/`, `cryptorand/`, `Dockerfile.websrv` — kept and
  modified.

### Dependency cleanup that falls out

`go.mod` loses `cloud.google.com/go`, `code.sajari.com/word2vec`,
`google.golang.org/api`, `google.golang.org/genproto`, `golang.org/x/net`,
`github.com/olekukonko/tablewriter`, `github.com/ziutek/blas` and their
transitive tails. Remaining direct deps: `gorilla/mux`, `gorilla/websocket`,
`gorilla/securecookie`, `rs/cors`, `peterbourgon/ff/v4`, `modernc.org/sqlite`,
`google/go-cmp`, `golang.org/x/text`.

---

## 4. Draft protocol — `codenames-bot/1`

Transport: one WebSocket per bot to `wss://<host>/bot/v1/ws`. Text frames, one
JSON object per frame. Version lives in the path *and* is confirmed in the
handshake; a v2 would be served at `/bot/v2/ws` with v1 left running, so a
mid-semester change cannot break every student's bot at once.

Every message has a `"type"`. Server→bot messages that need an answer carry a
`"request_id"` the bot must echo. Unknown fields are ignored in both directions
(forward compatibility); unknown `type` from a bot is an error.

### 4.1 Shared: the state object

Sent as its own `state` message and embedded in every request. `agent` is
`null` on a card whose identity the recipient is not entitled to know — for an
operative that means every unrevealed card; for a spymaster, never.

```json
{
  "game_id": "SmugglerCourtUnicorn",
  "move_number": 7,
  "status": "PLAYING",
  "active_team": "RED",
  "active_role": "SPYMASTER",
  "starting_team": "RED",
  "guesses_left": 0,
  "remaining": { "RED": 4, "BLUE": 6 },
  "board": [
    { "index": 0, "word": "CROWN", "revealed": false, "agent": "RED",  "revealed_by": null },
    { "index": 1, "word": "ICE",   "revealed": true,  "agent": "BYSTANDER", "revealed_by": "RED" }
  ],
  "history": [
    { "team": "RED", "clue": { "word": "MONARCH", "count": 2 },
      "guesses": [ { "word": "CROWN", "agent": "RED", "correct": true } ],
      "ended_by": "WRONG_GUESS" }
  ]
}
```

Enums, all strings: `team` ∈ `RED|BLUE`; `role` ∈ `SPYMASTER|OPERATIVE`;
`agent` ∈ `RED|BLUE|BYSTANDER|ASSASSIN|null`; `status` ∈
`LOBBY|PLAYING|FINISHED`; `ended_by` ∈
`COUNT_EXHAUSTED|WRONG_GUESS|PASS|FORFEIT|GAME_OVER`.

`board` is always all 25 cards in a stable index order, revealed and unrevealed
alike, so a bot can key off `index`.

### 4.2 Bot → server

**`hello`** — must be the first frame. Claims one or more seats.
```json
{ "type": "hello", "protocol_version": 1,
  "join_code": "Smuggler-Court-Unicorn",
  "bot_name": "ethan-qwen3",
  "seats": [ { "team": "RED", "role": "SPYMASTER" },
             { "team": "RED", "role": "OPERATIVE" } ] }
```
Join codes are matched case-insensitively with `-`, `_` and spaces stripped, so
`smugglercourtunicorn` works.

**`clue`** — answers a `clue_request`.
```json
{ "type": "clue", "request_id": "r-7", "word": "MONARCH", "count": 2 }
```

**`guess`** — answers a `guess_request`.
```json
{ "type": "guess", "request_id": "r-8", "word": "CROWN" }
```

**`pass`** — answers a `guess_request`, ending the turn.
```json
{ "type": "pass", "request_id": "r-8" }
```

**`resume`** — reconnect after a drop, in place of `hello`.
```json
{ "type": "resume", "protocol_version": 1, "resume_token": "rt_9f3a…" }
```

### 4.3 Server → bot

**`welcome`** — `hello`/`resume` accepted.
```json
{ "type": "welcome", "protocol_version": 1,
  "game_id": "SmugglerCourtUnicorn", "bot_id": "b_7Kq…",
  "resume_token": "rt_9f3a…",
  "seats": [ { "team": "RED", "role": "SPYMASTER" } ],
  "lobby": { "seats": [
      { "team": "RED",  "role": "SPYMASTER", "bot_name": "ethan-qwen3" },
      { "team": "RED",  "role": "OPERATIVE", "bot_name": "ethan-qwen3" },
      { "team": "BLUE", "role": "SPYMASTER", "bot_name": null },
      { "team": "BLUE", "role": "OPERATIVE", "bot_name": null } ],
    "all_filled": false },
  "limits": { "move_timeout_ms": 30000, "max_attempts_per_move": 3,
              "reconnect_grace_ms": 60000 } }
```

**`lobby_update`** — a seat was filled or vacated. Same `lobby` shape.

**`game_start`** — `{ "type": "game_start", "state": { … } }`, masked per
recipient.

**`state`** — an unsolicited full-state refresh (after reconnect, or after any
public event). Never needs a reply.

**`clue_request`**
```json
{ "type": "clue_request", "request_id": "r-7", "team": "RED",
  "attempt": 1, "attempts_remaining": 2,
  "deadline_ms": 30000, "max_count": 4, "state": { … } }
```

**`guess_request`**
```json
{ "type": "guess_request", "request_id": "r-8", "team": "RED",
  "attempt": 1, "attempts_remaining": 2, "deadline_ms": 30000,
  "clue": { "word": "MONARCH", "count": 2 },
  "guesses_left": 3, "may_pass": false, "state": { … } }
```
`may_pass` is `false` on the first guess after a clue — the official rules
require at least one guess — and `true` thereafter.

**`clue_given`**, **`guess_given`**, **`turn_end`** — public events, no reply.
```json
{ "type": "clue_given",  "team": "RED", "clue": { "word": "MONARCH", "count": 2 } }
{ "type": "guess_given", "team": "RED", "word": "CROWN",
  "card": { "index": 0, "word": "CROWN", "agent": "RED" },
  "correct": true, "can_keep_guessing": true }
{ "type": "turn_end", "team": "RED", "reason": "WRONG_GUESS", "next_team": "BLUE" }
```

**`error`** — your message was rejected.
```json
{ "type": "error", "request_id": "r-7", "code": "CLUE_ON_BOARD",
  "message": "clue \"KINGS\" shares a root with the board word \"KING\"",
  "retry": true, "attempts_remaining": 1 }
```
Codes, stable and documented: `BAD_JSON`, `UNKNOWN_TYPE`,
`PROTOCOL_VERSION_UNSUPPORTED`, `HELLO_REQUIRED`, `ALREADY_HELLO`,
`UNKNOWN_JOIN_CODE`, `GAME_ALREADY_STARTED`, `SEAT_TAKEN`, `INVALID_SEAT`,
`STALE_REQUEST`, `NOT_YOUR_TURN`, `WRONG_ROLE`, `CLUE_NOT_ONE_WORD`,
`CLUE_ON_BOARD`, `CLUE_COUNT_OUT_OF_RANGE`, `GUESS_NOT_ON_BOARD`,
`GUESS_ALREADY_REVEALED`, `PASS_NOT_ALLOWED`, `MISSING_FIELD`, `RATE_LIMITED`.
`retry: false` means the move is forfeit or the connection is closing.

**`game_over`**
```json
{ "type": "game_over", "winning_team": "BLUE",
  "reason": "ALL_AGENTS_FOUND",
  "state": { … },
  "board": [ /* all 25 cards, every agent revealed */ ],
  "forfeits": [ { "team": "RED", "role": "SPYMASTER", "count": 1 } ] }
```
`reason` ∈ `ALL_AGENTS_FOUND|ASSASSIN|FORFEIT_LIMIT|ABANDONED|HOST_CANCELLED`.

### 4.4 Schema artifacts

- `protocol/v1/*.schema.json` — JSON Schema for every message, which is what
  "validates every bot message against the schema" will literally mean: the
  referee's Go structs and the schemas are both generated from / checked against
  one source, and a schema test asserts they agree.
- `protocol/v1/README.md` — the human-readable spec, the document students read.
- `protocol/v1/testdata/*.json` — the shared golden fixtures (§6).

---

## 5. Referee: validation, timeouts, forfeits, logging

The referee is a per-game goroutine owning that game's state. It is the only
writer, which removes the current read-modify-write races around
`consensus`/`cluedelay` entirely.

### Validation, in order

Every bot frame: valid JSON → known `type` → schema-valid → `hello` done → for a
move, `request_id` matches the seat's outstanding request (else `STALE_REQUEST`,
which does **not** burn an attempt — a stale reply means the bot answered a move
that already resolved) → the seat is the active `team`+`role` (else
`NOT_YOUR_TURN` / `WRONG_ROLE`).

A **clue** must then satisfy:
1. `word`, trimmed, matches `^[A-Za-z]{2,}$` — one word, letters only, no
   spaces, hyphens, digits or punctuation (`CLUE_NOT_ONE_WORD`).
2. `word` is not an unrevealed board word or a derived form of one, via
   `codenames.ConflictingBoardWord` (`CLUE_ON_BOARD`).
3. `count` is an integer in `1 … (unrevealed cards belonging to the clueing
   team)` (`CLUE_COUNT_OUT_OF_RANGE`). See Q3 — this drops the official
   "zero"/"unlimited" clue, which the current engine encodes as `count == 0`.

A **guess** must satisfy: `word` normalises (case, separators) to a board card
(`GUESS_NOT_ON_BOARD`) that is not already revealed
(`GUESS_ALREADY_REVEALED`). A **pass** requires `may_pass` (`PASS_NOT_ALLOWED`).

Then the move goes through `game.Move()` unchanged — one authoritative rules
path, as today. Anything `Move()` rejects that the above missed is a referee
bug, logged loudly.

### Timeouts and retries

- **Per attempt:** `move_timeout_ms`, default **30 s** (`--move_timeout`),
  measured from the moment the request frame is written. Q4 — this may be too
  tight for a student's local Ollama.
- **Attempts:** `max_attempts_per_move`, default **3** (one try plus two
  retries). Each invalid move gets an `error` with `retry: true` and a *fresh*
  request (new `request_id`, `attempt` incremented) so a bot can never be
  confused about what it is answering.
- **Hard cap per move:** `max_attempts × move_timeout` wall clock, so a bot that
  answers at 29.9 s three times cannot stall a match indefinitely.
- **Forfeit the move** when attempts are exhausted or the hard cap elapses:
  - *Spymaster forfeits* → the team's turn ends with no clue; play passes. The
    server does **not** substitute a clue — picking a clue would be playing the
    game for the student, which would make the result meaningless.
  - *Operative forfeits* → treated as a pass, which ends the turn (and is
    allowed even on the first guess, since the alternative is a hung match).
  - Broadcast as `turn_end` with `reason: "FORFEIT"`, and recorded.
- **Repeated forfeits:** `max_consecutive_forfeits`, default **3** per seat →
  the match ends, `game_over` with `reason: "FORFEIT_LIMIT"`, the other team
  wins. This is the stop-the-world guard against a bot that is connected but
  brain-dead.
- **Disconnect:** `reconnect_grace_ms`, default **60 s**. The seat is held, the
  clock on any outstanding request is paused, and a `resume` with the
  `resume_token` from `welcome` gets the bot a `state` plus a re-sent request.
  This matters a lot when students are on dorm wifi. Grace expiry → `game_over`
  with `reason: "ABANDONED"`.
- **Rate limit:** a cheap token bucket per connection (default 20 frames / 10 s)
  so a looping bot gets `RATE_LIMITED` rather than taking the server down.

### Logging

One JSONL file per game at `<log_dir>/games/<game_id>.jsonl`, one record per
frame in **both** directions:

```json
{ "ts": "2026-10-06T20:14:03.411Z", "seq": 42, "dir": "in",
  "seat": { "team": "RED", "role": "SPYMASTER" }, "bot_name": "ethan-qwen3",
  "type": "clue", "request_id": "r-7",
  "payload": { "type": "clue", "request_id": "r-7", "word": "KINGS", "count": 2 },
  "verdict": "rejected", "error_code": "CLUE_ON_BOARD",
  "latency_ms": 1840 }
```

Plus a `match_result` record at the end (winner, reason, per-seat forfeit and
rejection counts, total latency stats). Exposed at
`GET /api/game/{id}/botlog` to the game's host and rendered in the spectator UI,
so a student can read exactly what their bot sent and exactly why it was
refused. This is the debugging surface requirement 3 asks for, and it replaces
the research logs being deleted — same mechanism, opposite purpose.

---

## 6. What I will add

### Backend

- **`protocol/v1/`** — the Go DTOs (`Envelope`, `Hello`, `ClueRequest`, `State`,
  `Error`, …), `MarshalJSON`/`UnmarshalJSON`, the error-code constants, the
  `codenames.*` ↔ DTO mappers, and the JSON Schemas. No dependency on `web/`,
  so the referee tests and the kit's fixture generator can both use it.
- **`referee/`** — one goroutine per game: owns state, drives
  request→validate→`game.Move()`→broadcast, runs the timers, counts attempts and
  forfeits, writes the game log. Pure Go, no HTTP, driven in tests by in-memory
  fake bots. This is where a full legal game, a clue-on-board rejection, a
  timeout forfeit and a forfeit-limit abandon all get unit tests.
- **`botws/`** (or `web/botws.go`) — the `/bot/v1/ws` endpoint: upgrade,
  `hello`/`resume` handshake, seat claim, bind the connection to the game's
  referee. Extends `hub/` with **inbound** routing (`readPump` currently
  discards frames) and per-connection send queues; `maxMessageSize` goes from
  512 bytes to ~64 KB since a `state` frame with 25 cards and history is larger
  than that.
- **`lobby/`** — the four-seat model, join-code normalisation and lookup, seat
  claim/release, host start. Backed by `codenames.DB` so a restart mid-match
  does not lose the game.
- **`web/`** changes — delete the AI-request, clue-hold, admin and CSV-log
  handlers listed in §3b; add `GET /api/game/{id}/botlog`; add static-file
  serving of the built SPA with a `200.html` SPA fallback (needed for one URL on
  Fly — see §1); keep the browser's cookie+REST path for the host and for human
  seats (Q2).
- **`cmd/codenames-server/`** — new flags: `--move_timeout`,
  `--max_attempts_per_move`, `--max_consecutive_forfeits`,
  `--reconnect_grace`, `--auto_start`, `--static_dir`. Drop
  `--ai_server_endpoint`, `--admin_secret`, `--clue_delay`.
- **`cmd/referee-local/`** — a tiny binary that creates one game, waits for four
  seats, auto-starts, plays, prints the result, exits. This is what CI uses for
  the end-to-end test and what a student can run to self-host a match.

### Frontend (host + spectator, assuming Q2 = keep)

Rework `SetupView` into a bot lobby: the join code shown large and copyable,
four seats with the connected bot's name and live status (connected / thinking /
timed out / forfeited), kick, and start. `BoardView` becomes a spectator board
with full colours for the host. New: a bot log panel showing the per-game JSONL
with rejections highlighted. Delete `ClueHoldToggle` and the `/admin` route.

### Python kit — `kit/`, package `codenames-bot`

```
kit/
  pyproject.toml                 # hatchling; one runtime dep: websockets
  README.md                      # the 10-minute path
  src/codenames_bot/
    __init__.py                  # Bot, Clue, Guess, PASS, run
    protocol.py                  # v1 dataclasses, parse/serialise, enums
    views.py                     # SpymasterView / OperativeView
    client.py                    # WS loop: hello, dispatch, reply, reconnect
    referee.py                   # offline referee for local play
    runner.py                    # bot-vs-bot offline match + pretty printer
    cli.py                       # `codenames-bot play …`
  examples/random_bot.py
  examples/ollama_bot.py
  tests/                         # incl. the shared golden fixtures
```

The base class is the whole API a student sees:

```python
from codenames_bot import Bot, Clue, Guess, PASS, run

class MyBot(Bot):
    def give_clue(self, view) -> Clue:
        # view.my_words, view.opponent_words, view.bystanders, view.assassin,
        # view.unrevealed, view.remaining, view.history
        return Clue(word="MONARCH", count=2)

    def make_guess(self, view) -> Guess:
        # view.clue, view.guesses_left, view.unrevealed, view.may_pass, view.history
        return PASS if view.may_pass and not view.clue else Guess(view.unrevealed[0])

if __name__ == "__main__":
    run(MyBot())
```

The views are where the kit earns its place: a student should never filter a
list of card dicts by an agent string to find their own words.

A bot subclasses `Bot` once and gets both of its team's seats, matching the
two-students-per-match format: `give_clue` is called when it is that team's
spymaster turn, `make_guess` when it is their operative turn. The kit claims
both seats in `hello` by default; `run(MyBot(), seats="red:spymaster")` narrows
it to one seat if the format ever changes.

`examples/random_bot.py` picks a random dictionary word that is not on the board
and a random legal count, then guesses random unrevealed cards — a legal,
terrible baseline and the opponent for offline testing.
`examples/ollama_bot.py` is ~60 lines against a local Ollama `/api/chat` with a
JSON-shaped prompt, and deliberately shows the retry path (what to do when the
server says `CLUE_ON_BOARD`).

**On the offline runner duplicating the rules:** `referee.py` is a second
implementation of the referee in Python, and second implementations drift. I
propose keeping it honest with **shared golden fixtures**:
`protocol/v1/testdata/*.json` holds `(state, move) → (verdict, error_code)`
cases, generated from the Go referee and asserted by both the Go test suite and
the kit's `pytest`. Drift becomes a failing test in CI rather than a student
being surprised on match day. The alternative — making the offline runner call a
real Go server — would mean every student installs Go, which I think is the
worse trade. Flagging it as a judgment call rather than burying it.

README 10-minute path: install → copy `random_bot.py` → `codenames-bot play
--offline --opponent random` (a full game in the terminal, no server) → `--code
Smuggler-Court-Unicorn` against the real server → edit two methods.

### Hosting — separate Fly app

`fly.competition.toml` with `app = "codenames-bots"` (study app, whenever it gets
one, must use a different `app` name — different app means a different `*.fly.dev`
hostname and a different volume, so no shared URL and no shared DB by
construction):

- One machine, `min_machines_running = 1`, `auto_stop_machines = false`. The
  WebSocket hub, the referee goroutines and SQLite are all in-process; this
  cannot horizontally scale, and pretending otherwise would silently break
  matches.
- A 1 GB volume at `/data` holding `codenames.db`, `hashKey`, `blockKey` and
  `logs/`.
- `Dockerfile.competition` — multi-stage: build the SPA with pnpm, build the Go
  server, copy both into the final image (so one URL serves the API, the
  WebSocket and the UI). `Dockerfile.websrv` as it stands builds neither the
  frontend nor the new packages, so this is a new file rather than an edit.
- Health check on `GET /healthz` (new, trivial).
- `docs/DEPLOY.md` — the deploy runbook and the explicit statement that the two
  apps share nothing.

---

## 7. Phases

Each phase ends with `go build ./...`, `go vet ./...`, `go test ./...`,
`pnpm run check && pnpm run lint` where the frontend changed, `pytest` where the
kit changed, and one commit. All on `codenames-competition`.

**Phase 1 — Cleanup. ✅ DONE** (`ac4f8b0`, 80 files, +1,171 / −61,742).
Deleted §3 in full. `go mod tidy` left 8 direct dependencies. Rewrote
`.gitignore`, `CLAUDE.md`, `AGENTS.md`, `README.md`, `web/README.md`,
`docker-compose.yml` and `Dockerfile.websrv` (which now builds the SPA into the
server image, so one URL serves everything).

Three changes beyond pure deletion, each forced by a removal:
- `serveClue` applies and broadcasts a clue immediately, in place of the
  `cluedelay` hold.
- `serveGuess` applies a confirmed guess directly, in place of the `consensus`
  majority vote — one operative per seat means there is nobody to agree with.
- Players choose their own display name, in place of `names.Random`. Knowing
  whose bot is whose is the point here, the opposite of the study's need.

Also fixed a pre-existing bug: a stray `flag.Parse()` after `ff.Parse()`
re-parsed `os.Args` against the global flagset, so the server rejected *any*
command-line flag and only ever worked through environment variables. The
compose file and the Fly deploy both pass flags, so this had to go.

*Verified:* `go build ./...`, `go vet ./...`, `go test ./...` all pass (the
pre-existing `llm` failure left with that package); `svelte-check` reports 0
errors; and a full game plays start to finish over real HTTP — four players
join, roles assign, a board word is refused as a clue with a clear 400, a legal
clue lands, guesses reveal cards, and the game ends with a winner after 17
moves.

`pnpm run lint` still reports 6 errors, all inherited from `master` and all in
lines this work did not touch (`svelte/prefer-svelte-reactivity` ×3,
`no-explicit-any`, `prefer-const`, `no-navigation-without-resolve`). Phase 1
removed one such error and added none. Flagged rather than fixed: two of the six
would change runtime behaviour, which does not belong in a cleanup commit.

**Phase 2 — Protocol + referee.** `protocol/v1/` with schemas, `referee/`,
golden fixtures. *Done when:* unit tests play a full legal game through the
referee with fake bots, and cover each rejection code, a timeout forfeit, a
forfeit-limit abandon, and a reconnect-with-resume.

**Phase 3 — Lobbies + transport.** `lobby/`, `botws/`, bidirectional `hub/`,
`/bot/v1/ws`, per-game JSONL logging, `/api/game/{id}/botlog`, the frontend
lobby and spectator rework, `cmd/referee-local`. *Done when:* two Go test bots
join by code over real WebSockets and finish a game, and a deliberately broken
bot gets the right `error` code and a forfeit.

**Phase 4 — Python kit.** The package, both examples, the offline runner, the
README, `pytest` against the shared fixtures. *Done when:* `codenames-bot play
--offline` finishes a game, and two example bots on the local server finish a
game by join code.

**Phase 5 — Deploy.** `Dockerfile.competition`, `fly.competition.toml`,
`/healthz`, `docs/DEPLOY.md`, deploy, smoke test. *Done when:* the two example
bots, run from two different machines, join `codenames-bots.fly.dev` by code and
finish a legal game; a broken bot is rejected with a clear error; all tests
pass.

I will not run `fly deploy` or push anything outward without asking you first.

---

## 8. Open questions

**Q1 — Competition format. RESOLVED: two students per match.** Each bot plays
both roles for one team (spymaster + operative), so a match is student A's bot
vs student B's bot. The protocol still lets one connection claim one *or more*
seats in `hello`, so a four-student-per-match format needs no protocol change if
Mourning wants it different — only a lobby policy flag. The kit's base class and
README assume per-team.

**Q2 — Keep the browser frontend. RESOLVED: keep it.** Reworked as a host
lobby + spectator board + bot-log viewer, so people can watch the bots compete.
Spectating is a first-class feature, not a leftover: it is also how a student
sees that their bot is clueing badly rather than crashing.

**Q3 — Clue counts.** Official Codenames allows "zero" and "unlimited". The
current engine encodes `count == 0` as unlimited guesses. I propose restricting
to `1 … (your unrevealed cards)`, which removes an edge case that is awkward to
referee and that no student will ask for. Keep the official behaviour instead?

**Q4 — Move timeout.** 30 s default. A student's local Ollama on a laptop can
take 60 s+ for a clue, so this may be the single most annoying number in the
system. Options: raise the default to 60 s; or keep 30 s and make it per-game so
you can relax it for a demo. Which?

**Q5 — Existing study data. RESOLVED: backed up, safe to delete.** The logs
are saved to a separate folder outside this branch, so Phase 1 deletes `logs/`
(`ai_reasoning.jsonl`, `all_games.csv`, `game-SmugglerCourtUnicorn.csv`) along
with the code that writes it. I will confirm the backup exists on disk before
deleting anything. `UPDATES.txt` and `docs/original_design.md` stay unless you
say otherwise; `docs/LAUNCH_PLAN.md` goes, as study-specific.

**Q6 — Human seats.** Should the competition server still allow a *person* to
take a seat in the browser (play against your own bot)? Nearly free if we keep
the frontend, and pedagogically useful. Bots-only is simpler and removes the
whole cookie-auth path.

**Q7 — Tournament machinery.** Requirement 1 is lobbies, not brackets. I am
treating scheduling, a leaderboard and Elo as **out of scope for v1**, with the
`match_result` log record designed so a leaderboard can be built on top later.
Confirm, or tell me it is in scope.

**Q8 — Fly.io reality. RESOLVED: nothing to collide with, build fresh.** No
existing deployment constrains any decision, so the tournament deploy is set up
from scratch: a new `codenames-bots` Fly app, its own volume, its own
`Dockerfile.competition` that builds the SPA and the Go server into one image
serving one URL. I will not run `fly deploy` without asking.

**Q9 — Your uncommitted `llm/llm.go`. RESOLVED: committed, preserved off this
branch.** The +89/-6 of prompt tuning is commit `8714b66` on a new branch
`study/llm-prompt-tuning`, cut from `master`, authored as you with no
co-author trailer. It is **not** on `master`: the hard rule for this work is
never to commit to, merge into, rebase or push `master`, so the commit sits one
branch away and merging it is your call —
`git checkout master && git merge --ff-only study/llm-prompt-tuning`. The
competition branch's working tree is clean and matches `master` for that file,
so Phase 1 can delete `llm/` without losing the tuning.

**Q10 — Kit distribution.** PyPI (`pip install codenames-bot`), a git clone of
`kit/`, or a wheel you hand out? Affects the README's first line. Default
assumption: `pip install -e kit/` from a clone, documented so PyPI is a later
drop-in.

**Q12 — Scrubbing the attribution from GitHub history. BLOCKED, needs your
call.** Two different things are bundled in "wipe your name":

*New commits* — done, and it costs nothing. Commit `8714b66` carries no
`Co-Authored-By` trailer and is authored as `EthanC306 <ec036824@ohio.edu>`.
Every commit I make from here will be the same.

*The 13 commits already in history* — not done, and I did not want to do it
unasked. These carry a `Co-Authored-By: Claude` trailer and are all pushed to
`origin/master`:

```
460d0c5 2026-08-03  Add score context to spymaster prompt, and a turn history dropdown
d3877a3 2026-08-03  Tag each merged entry with the model that produced it
a2c569f 2026-08-02  Fix raw-attempt misattribution in merge_reasoning_data.py
9a1e14c 2026-08-02  Add --watch mode to merge_reasoning_data.py
8b9bf43 2026-08-02  Add --games N option to merge_reasoning_data.py
14410c0 2026-08-02  Add human-readable transcript output to merge_reasoning_data.py
8e92d8c 2026-08-02  Add script to merge AI reasoning log with raw model responses
deef8f1 2026-07-29  Add docker-compose local dev setup
844b46f 2026-06-10  Let AI operatives pass instead of forcing low-confidence guesses
615b28a 2026-06-10  Assign random generated usernames to all players
a9fe4fe 2026-06-10  Add UPDATES.txt project log with to-do list
5411d36 2026-06-10  Add logs/ to .gitignore
fdd53a9 2026-06-10  Make AI players harder to detect in Turing test games
```

There is no tool that edits a pushed commit message in place. Removing those
trailers means rewriting history from `fdd53a9` (2026-06-10) forward and
force-pushing, which:

- **rewrites `master`** — directly against this project's hard rule;
- **force-pushes to a shared GitHub repo.** `abramceastham-byte/ai_codenames`
  has an active collaborator (Abram Eastham, 15+ commits). A force-push breaks
  their clone and anyone else's; they would have to re-clone or hard-reset;
- **changes every commit hash** from June onward, orphaning any existing links,
  PR references or branch bases that point at them. `origin/feature/details`,
  `origin/feature/log`, `origin/feature/log-export` and `origin/llm-changes` all
  branch off that range.

I have deliberately not attempted it, and my attempts to even *probe* for the
rewrite tooling were blocked by this environment's git safety classifier, which
I take as the right default. Options, cheapest first:

1. **Do nothing to history.** New commits are clean; the old trailers stay. No
   risk, nothing breaks.
2. **Rewrite only the competition branch's future.** Already the case.
3. **Full rewrite + force-push.** You tell Abram first, both of you re-clone
   afterwards, and you confirm explicitly that you want `master` rewritten
   despite the hard rule. I will walk through it with you rather than run it
   unattended, and I would want the repo mirrored to a backup remote before
   touching anything.

My recommendation is (1), unless the attribution is a specific problem for how
this work is being submitted or graded — in which case say so and we do (3)
carefully.

**Q11 — Bot identity and trust.** Join code plus a self-declared `bot_name`, or
do students get a per-student token so a bot cannot impersonate another and the
logs are reliably attributable? **Recommendation:** join code only for v1 — this
is a classroom, and a token adds an issuance workflow — with the protocol
reserving an optional `"auth_token"` field in `hello` so it can be turned on
later without a v2.

---

## 9. What "done" looks like, concretely

- Two example bots, on two different machines, join the same game by code and
  finish a full legal game. *(Phase 5 smoke test, plus a CI test that does the
  same against a local server.)*
- A deliberately broken bot is rejected with a clear error. *(Covered for each
  of: non-JSON, unknown type, clue on the board, two-word clue, out-of-range
  count, guess of a revealed card, guess out of turn, illegal pass, silence
  until timeout. Each asserts a specific `code` plus the human `message`, and
  each is in the referee test suite.)*
- `go build ./...`, `go vet ./...`, `go test ./...`, `pnpm run check`, `pnpm run
  lint`, and `pytest` all pass. The one pre-existing `llm` test failure is
  resolved by that package's deletion in Phase 1, so green means green.

---

**Status.** Q9 is done: your prompt tuning is committed to
`study/llm-prompt-tuning` and the competition branch's working tree is clean. No
other code has been created, modified or deleted — `PLAN.md` is the only
addition to this branch.

**Waiting on one thing before Phase 1:** an explicit go-ahead to start deleting,
since §3 removes ~7,000 lines and ~30 MB of tracked binaries. The six remaining
open questions all have recommendations I can proceed on, so they do not need to
block; Q12 does need an answer, because it is the only item that would touch
`master` and GitHub.

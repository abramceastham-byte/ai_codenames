# Codenames Bot Tournament

This branch is a stripped-down version of our Codenames project for the class
bot tournament. Students write a Codenames bot (a spymaster and an operative),
and matches are run **locally on one machine**. There is no hosting, and no
accounts beyond a name typed into the browser.

What's here:

- `cmd/codenames-server`: game server (REST + WebSocket, stores games in SQLite)
- `frontend/`: browser UI for setting up and watching a game
- `cmd/ai-server`: hosts the bots and joins them to games
- `game/`, `codenames/`: the rules engine and core types

## Install

You need **Go 1.24+**, **Node 20+**, and **pnpm** (`corepack enable` provides it).

```bash
git clone <repo-url> && cd ai_codenames
cd frontend && pnpm install && cd ..
go build ./...
```

## Where the bot interface is defined

Students write bots as **standalone executables in any language**. For each
move, the engine starts the bot, writes one JSON request to its stdin, and reads
one JSON response from its stdout.

- [`BOT_GUIDE.md`](BOT_GUIDE.md): the guide for students (examples, legal-clue rules, timeouts, forfeits)
- [`botproto/schema.json`](botproto/schema.json): the formal JSON Schema
- [`botproto/botproto.go`](botproto/botproto.go): the same protocol as Go structs, plus clue validation
- [`botproto/match.go`](botproto/match.go): `RunMatch` plays one game between two students' bots
- [`examples/python/`](examples/python): a minimal spymaster and operative to use as a template

`go test ./botproto/` plays full games between the example bots. It needs `python3`.

The browser flow (see [Watching a game in the browser](#watching-a-game-in-the-browser)) uses the older in-process Go bot interface
(`Spymaster`/`Operative` in [`codenames/codenames.go`](codenames/codenames.go),
registered in [`cmd/ai-server/main.go`](cmd/ai-server/main.go)). It does not
run JSON bots yet.

## Running one match between two bots

`cmd/match` plays one game between two teams of JSON bots. Each team needs a
spymaster command and an operative command. A command is split on spaces and
run directly, not through a shell, so **paths in a bot command can't contain
spaces** (quoting doesn't help). Move or rename the bot's folder if needed.
Run everything from the repo root.

Try it with the example Python bots playing each other:

```bash
go run ./cmd/match \
  --red_name alpha \
  --red_spymaster "python3 examples/python/spymaster.py" \
  --red_operative "python3 examples/python/operative.py" \
  --blue_name beta \
  --blue_spymaster "python3 examples/python/spymaster.py" \
  --blue_operative "python3 examples/python/operative.py"
```

It prints a summary:

```
Match:    alpha (red) vs beta (blue), blue starts, seed 42
Winner:   beta (blue)
Ended by: all agents found
Turns:    21
Forfeits: none
Log:      logs/2026-10-20_203542_alpha-vs-beta.jsonl
```

For a student's bot, point the commands at their files (any language works):

```bash
go run ./cmd/match \
  --red_name alice \
  --red_spymaster "python3 bots/alice/spymaster.py" \
  --red_operative "python3 bots/alice/operative.py" \
  --blue_name bob \
  --blue_spymaster "node bots/bob/spymaster.js" \
  --blue_operative "./bots/bob/operative"
```

Optional flags:

| flag | default | meaning |
|---|---|---|
| `--seed N` | random | Board and starting team. The summary prints the seed, so passing it again replays the same board. The bots' own choices may still differ between runs. |
| `--move_timeout 10s` | `10s` | Time limit for each bot move |
| `--max_turns 50` | `50` | Turn limit. If it's reached, the team with more forfeits loses. If forfeits are equal, the team with fewer cards left wins. The `result` log line's `tiebreak` field (`forfeits` or `cards_left`) says which rule decided it. |
| `--log_dir logs` | `logs` | Where logs and results are written |

### Logs and results

All output goes to `logs/`, which git ignores:

- **`logs/<date>_<time>_<red>-vs-<blue>.jsonl`**: a move log for each match, one JSON object per line.
  The first line is `match_start` (teams, bot commands, seed, the starting board with every card's type).
  After that come `clue`, `guess` (with what each guess revealed), `forfeit` (the reason plus the bot's stderr),
  and `turn_end` lines, ending with `result`.
- **`logs/results.csv`**: one row per match (`time, red_team, blue_team, winner, turns, forfeits, reason, log_file`),
  for tracking wins across the tournament. Open it in any spreadsheet program.

## Watching a game in the browser

This uses the older Go bots, not JSON bots. Open three terminals in the repo root.

```bash
# 1. Game server (port 8080)
AUTH_SECRET=abc123 AI_SERVER_ENDPOINT=http://localhost:8081 go run ./cmd/codenames-server/

# 2. Bot server (port 8081)
AUTH_SECRET=abc123 WEB_SERVER_ENDPOINT=http://localhost:8080 go run ./cmd/ai-server/

# 3. Frontend (port 5173)
cd frontend && pnpm run dev
```

Then:

1. Open http://localhost:5173, enter any name, and create a game.
2. For each of the four seats, click **Add AI** and pick a bot from the dropdown
   (for example, `team-alpha` for both Red seats and `team-beta` for both Blue seats).
3. Start the game. The bots play automatically. The board and game log update live.

When a game finishes, its log is written as CSV to `logs/` (use `--log_dir` to change this).

### Quick sanity check without bots

`./cmd/codenames-local/run_local.sh` plays a game in the terminal, with all four
roles typed on stdin.

## Development

```bash
go build ./... && go vet ./... && go test ./...
cd frontend && pnpm run check
```

# Codenames Bot Tournament

Students build Codenames bots, each team a **spymaster** and an **operative**, and you run matches
between teams on your own machine. Bots are standalone programs in any language that read a JSON request
on stdin and write a JSON response on stdout. Every match produces a move log you can replay, and a row
in a results file.

## 1. Install

You need **Go 1.24+** ([go.dev/dl](https://go.dev/dl/)), plus whatever the students' bots are written in
(e.g. **Python 3**). Node isn't needed.

```bash
git clone <repo-url> && cd ai_codenames
go build ./...
```

## 2. Run a match

From the repo root:

```bash
go run ./cmd/match \
  --red_name test_student \
  --red_spymaster "python3 students/test_student/spymaster.py" \
  --red_operative "python3 students/test_student/operative.py" \
  --blue_name test_student_2 \
  --blue_spymaster "python3 students/test_student_2/spymaster.py" \
  --blue_operative "python3 students/test_student_2/operative.py"
```

```
Match:    test_student (red) vs test_student_2 (blue), blue starts, seed 42
Winner:   test_student (red)
Ended by: all agents found
Turns:    18
Forfeits: none
Log:      logs/2026-10-20_210927_test_student-vs-test_student_2.jsonl
```

Each bot command is run directly, not through a shell, and **paths can't contain spaces**.
On Windows, use `python` or `py` instead of `python3`, and put the whole command on one line.

| Optional flag | Default | Meaning |
|---|---|---|
| `--seed N` | random | Fixes the board and starting team. The seed is printed, so you can rerun the same board. |
| `--move_timeout 10s` | `10s` | Time limit for each bot move |
| `--max_turns 50` | `50` | Turn limit. At the limit, the team with more forfeits loses. If forfeits are equal, fewer cards left wins. |
| `--log_dir logs` | `logs` | Where logs and results go |

A bot that crashes, times out, sends bad JSON, or makes an illegal move **forfeits that turn**. The match
always finishes, and the summary lists each forfeit with its reason.

## 3. Results and logs

Everything goes to `logs/`, which git ignores:

- **`logs/results.csv`**: one row per match (`time, red_team, blue_team, winner, turns, forfeits, reason, log_file`).
  Open it in a spreadsheet to track wins.
- **`logs/<date>_<time>_<red>-vs-<blue>.jsonl`**: the full move log, one JSON object per line.
  It starts with the teams, bot commands, seed, and starting board. Then come the clues, guesses (with what
  each revealed), and forfeits (with the bot's error output). It ends with the result.

## 4. Replay a game

```bash
go run ./cmd/replay logs/2026-10-20_210927_test_student-vs-test_student_2.jsonl
```

This shows the board in the terminal and steps through the game one event at a time.
Controls: **Enter** next, **b** back, **s** switch between the operative's view and the spymaster's view, **q** quit.

To play it hands-free (useful for showing a game in class):

```bash
go run ./cmd/replay --auto --delay 500ms --spymaster logs/<file>.jsonl
```

Add `--no-color` if colors look wrong (e.g. in an older Windows console).

## 5. Student bots

Each team submits its bot as a folder: **`students/<team>/`**, containing a spymaster and an operative
(e.g. `spymaster.py` and `operative.py`). Folder names shouldn't contain spaces.

- **[BOT_GUIDE.md](BOT_GUIDE.md)**: everything students need, including the protocol, legal clues, timeouts, fair-play rules, and how to test.
- **[examples/](examples)**: minimal Python bots, the official word list, and sample requests.
- **[students/test_student](students/test_student)** and **[students/test_student_2](students/test_student_2)**: two complete example teams with different strategies.

For developers: the protocol is defined in [botproto/](botproto) (Go structs, a JSON Schema, and the match runner).
Run `go test ./...` to check everything. The bot tests need `python3`.

## Legacy browser UI (not used for the tournament)

The original web version of the game is still here: a game server (`cmd/codenames-server`), a Svelte frontend
(`frontend/`), and a bot server (`cmd/ai-server`). It uses an older in-process Go bot interface
(`codenames/codenames.go`, registered in `cmd/ai-server/main.go`) and **can't run student JSON bots**.
It needs Node 20+ and pnpm as well as Go.

```bash
cd frontend && pnpm install && cd ..
AUTH_SECRET=abc123 AI_SERVER_ENDPOINT=http://localhost:8081 go run ./cmd/codenames-server/   # terminal 1
AUTH_SECRET=abc123 WEB_SERVER_ENDPOINT=http://localhost:8080 go run ./cmd/ai-server/         # terminal 2 (needs a registered Go bot)
cd frontend && pnpm run dev                                                                   # terminal 3, then open http://localhost:5173
```

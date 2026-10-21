# Codenames Bot Tournament

Students build Codenames bots, each team a **spymaster** and an **operative**, and you run matches
between teams on your own machine. Bots are standalone programs in any language that read a JSON request
on stdin and write a JSON response on stdout. Every match produces a move log you can replay, and a row
in a results file.

## 1. Install

You need **Go 1.24+** ([go.dev/dl](https://go.dev/dl/)), plus whatever the students' bots are written in
(e.g. **Python 3**). Node isn't needed.

```bash
git clone -b tournament-branch https://github.com/abramceastham-byte/ai_codenames.git
cd ai_codenames
go build ./...
```

## 2. Run the tournament

Each team is a folder in `students/`. To play every pair of teams twice (once with each color) and
print the standings, run this from the repo root:

```bash
go run ./cmd/tournament
```

```
Tournament: 2 teams, 2 matches, base seed 42

[1/2] test_student (red) vs test_student_2 (blue): test_student won, all agents found, 18 turns
[2/2] test_student_2 (red) vs test_student (blue): test_student_2 won, all agents found, 14 turns

Rank  Team            Played  Wins  Losses  Draws  Forfeits
1     test_student    2       1     1       0      0
1     test_student_2  2       1     1       0      0
```

Both games of a pair use the same board, so each team gets the starting side once. Use `--teams a,b,c` to
run only some teams.

Before playing anything, the tournament checks every team and **stops with a clear error** if a folder or
bot program is missing (e.g. `team "alice" spymaster: file "spymaster.py" not found`). Fix it and rerun.

## 3. Run one match

```bash
go run ./cmd/match test_student test_student_2
```

```
Match:    test_student (red) vs test_student_2 (blue), red starts, seed 1792545697580022634
Winner:   test_student_2 (blue)
Ended by: all agents found
Turns:    16
Forfeits: none
Log:      logs/2026-10-20_212137_test_student-vs-test_student_2.jsonl
```

The first team plays red. Rerun with `--seed` and the printed seed to replay the same board.
You can still give bot commands directly instead of team names (`--red_spymaster "..."` etc.; see `go run ./cmd/match -h`).

Optional flags for both commands:

| Flag | Default | Meaning |
|---|---|---|
| `--seed N` | random | Fixes the boards. For a tournament, pair *i* uses seed *N + i*. |
| `--move_timeout 10s` | `10s` | Time limit for each bot move |
| `--max_turns 50` | `50` | Turn limit. At the limit: a draw if no cards were revealed, otherwise more forfeits loses, then fewer cards left wins. |
| `--log_dir logs` | `logs` | Where logs and results go |
| `--students_dir students` | `students` | Where the team folders are |

A bot that crashes, times out, sends bad JSON, or makes an illegal move during a game **forfeits that turn**.
The match always finishes, and each forfeit is listed with its reason.

## 4. Results and logs

Everything goes to `logs/`, which git ignores:

- **`logs/results.csv`**: one row per match (`time, red_team, blue_team, winner, turns, forfeits, reason, log_file`).
  Open it in a spreadsheet to track wins.
- **`logs/<date>_<time>_<red>-vs-<blue>.jsonl`**: the full move log, one JSON object per line.
  It starts with the teams, bot commands, seed, and starting board. Then come the clues, guesses (with what
  each revealed), and forfeits (with the bot's error output). It ends with the result.

## 5. Replay a game

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

## 6. Student bots

Each team submits a folder **`students/<team>/`**. The folder name is the team name, and it shouldn't
contain spaces.

- **Python bots** just need `spymaster.py` and `operative.py` in the folder. They're run with `python3`,
  or `python` if `python3` isn't installed.
- **Other languages** add a `team.json` with the two commands, which run from inside the team folder:
  ```json
  {"spymaster": "node spymaster.js", "operative": "./operative"}
  ```

Links for students:

- **[BOT_GUIDE.md](BOT_GUIDE.md)**: everything students need, including the protocol, legal clues, timeouts, fair-play rules, and how to test.
- **[examples/](examples)**: minimal Python bots, the official word list, and sample requests.
- **[students/test_student](students/test_student)** and **[students/test_student_2](students/test_student_2)**: two complete example teams with different strategies.

For developers: the protocol is defined in [botproto/](botproto) (Go structs, a JSON Schema, and the match runner),
and team loading and match output are in [arena/](arena).
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

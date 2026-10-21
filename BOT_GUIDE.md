# Codenames Bot Guide

You'll build **two programs**: a **spymaster** that gives clues and an
**operative** that guesses. They can be written in any language and can use
any approach: your own code, a local model through Ollama, or an outside API.
The only requirement is that they follow the protocol below.

**Start from the examples** in [`examples/`](examples):

- [`examples/python/spymaster.py`](examples/python/spymaster.py) and
  [`operative.py`](examples/python/operative.py): minimal bots that follow the protocol and play legal (random) moves
- [`examples/wordlist.txt`](examples/wordlist.txt): **the official word list**. Every board word comes from these 400 words.
- [`examples/sample_spymaster_request.json`](examples/sample_spymaster_request.json) and
  [`sample_operative_request.json`](examples/sample_operative_request.json): complete 25-card requests
  from a real game, for testing your bot by hand (see [Test your bot](#test-your-bot))

The formal definition of the protocol is [`botproto/schema.json`](botproto/schema.json), and you're
welcome to read it. If this guide and the schema ever disagree, the schema is correct.

## How your bot is run

For **every move**, the engine:

1. starts your program (a new process each time),
2. writes **one JSON object** to its **stdin**, then closes stdin,
3. reads **one JSON object** from its **stdout**,
4. waits for the program to exit.

- You have **10 seconds** per move, counted from start to exit. The professor may change this for the tournament.
- Write **only** the JSON response to stdout. Send debug output to **stderr**.
  The engine saves stderr in the match log **only when a move fails** (a crash, timeout, bad JSON, or illegal move).
  To see debug output from moves that succeed, run your bot by hand (see [Test your bot](#test-your-bot)).
- Your program is stateless between moves. Everything you need is in the request, including the full history of the game.
- Responses larger than 64 KB are rejected.
- Your bot runs with **your team folder as the working directory**, so `open("words.txt")` finds a file
  next to your script. To be safe no matter how it's started, you can build paths from the script's own
  location instead: in Python, `os.path.join(os.path.dirname(os.path.abspath(__file__)), "words.txt")`.

## Submitting your team

Your team is one folder, **`students/<team>/`**. The folder name is your team name, and it can't contain spaces.

**Python:** put `spymaster.py` and `operative.py` in the folder. That's all. They're run with
`python3` (or `python` if `python3` doesn't work on the machine).

```
students/my_team/
  spymaster.py
  operative.py
  words.txt        <- any files your bots need
```

**Any other language** (or Python with a different file layout): add a **`team.json`** with the command
for each bot. Commands are split on spaces and run from inside your team folder, not through a shell:

```json
{"spymaster": "node spymaster.js", "operative": "./operative"}
```

Other examples: `"java -jar spymaster.jar"`, `"./target/release/spymaster"`, `"python3 src/spymaster.py"`.
If you ship a compiled program, make sure it runs on the professor's machine, or ask what they'll use.

If your folder is missing a file or names a program that isn't installed, the match won't start. The error
names your team and the missing file or program, so check it before you submit (see [Test your bot](#test-your-bot)).

## The board

There are 25 cards, listed row by row (index 0 is top-left, index 24 is bottom-right).
Every card has a hidden type: `red`, `blue`, `bystander`, or `assassin`.
The team that starts (`starting_team` in every request) has 9 cards, the other team has 8,
and there are 7 bystanders and 1 assassin.

Board words always come from [`examples/wordlist.txt`](examples/wordlist.txt), and are always **lowercase**.
Four of them are two words joined by an **underscore**: `ice_cream`, `loch_ness`, `new_york`, and `scuba_diver`.
Guess them exactly as written, with the underscore. The clue overlap rule below applies to them too,
so `cream`, `york`, and `diver` are illegal clues while those cards are unrevealed.

A team wins by revealing all of its own cards. A team **loses immediately** if it reveals the assassin.

## Spymaster

### Request

The spymaster sees the type of **every** card.

```json
{
  "protocol_version": 1,
  "role": "spymaster",
  "team": "red",
  "starting_team": "blue",
  "board": [
    { "word": "africa", "type": "red", "revealed": false },
    { "word": "agent", "type": "blue", "revealed": true, "revealed_by": "blue" },
    { "word": "air", "type": "assassin", "revealed": false },
    { "word": "alien", "type": "bystander", "revealed": false }
  ],
  "history": [
    {
      "team": "blue",
      "clue": { "word": "spy", "count": 2 },
      "guesses": [
        { "word": "agent", "result": "blue" },
        { "word": "alien", "result": "bystander" }
      ],
      "end": "wrong_card"
    }
  ]
}
```

*(The board is cut to 4 cards here. Real requests always have 25.)*

### Response

```json
{ "clue": "continent", "count": 2 }
```

### What counts as a legal clue

A clue is legal only if **all** of the following are true:

1. It is **one word made only of the letters a–z** (any case). No spaces, hyphens, digits, or accents.
2. It is **1 to 30 characters** long.
3. `count` is a whole number **from 1 to 9**.
4. It does **not overlap any unrevealed board word**. That means the clue can't *be*
   a board word, *contain* one, or be *contained in* one, ignoring case.
   For example, if `snowman` is unrevealed, `snow`, `man`, `snowman`, and `snowmans` are
   all illegal. Words that are already revealed don't count.
   This is a plain letter-for-letter check: `snowmen` is allowed because `snowman`
   doesn't appear inside it.

An illegal clue **forfeits the turn** (see [Errors and timeouts](#errors-and-timeouts)).

The engine checks rules 1–4 automatically. The [fair play rules](#fair-play) below are checked by the professor.

## Operative

### Request

The operative sees the same board, but **`type` is missing on unrevealed cards**.
It also gets the clue for this turn and `guesses_allowed`, which is always **count + 1**.
That means one bonus guess, as in the board game. A spymaster may give a count larger than
the number of cards its team has left. It's legal, but it only gives the operative more chances to hit a wrong card.

```json
{
  "protocol_version": 1,
  "role": "operative",
  "team": "red",
  "starting_team": "blue",
  "board": [
    { "word": "africa", "revealed": false },
    { "word": "agent", "type": "blue", "revealed": true, "revealed_by": "blue" },
    { "word": "air", "revealed": false },
    { "word": "alien", "type": "bystander", "revealed": true, "revealed_by": "blue" }
  ],
  "history": [
    {
      "team": "blue",
      "clue": { "word": "spy", "count": 2 },
      "guesses": [
        { "word": "agent", "result": "blue" },
        { "word": "alien", "result": "bystander" }
      ],
      "end": "wrong_card"
    }
  ],
  "clue": { "word": "continent", "count": 2 },
  "guesses_allowed": 3
}
```

### Response

List your guesses **in the order you want them tried**:

```json
{ "guesses": ["africa", "asia"] }
```

You **must make at least one guess** every turn. An empty list (`{"guesses": []}`)
counts as a **forfeit**. To stop early, just list fewer guesses than `guesses_allowed`.

### How guesses are applied

The engine reveals your guesses one at a time, in order. It stops at the first of these:

- a guess reveals a card that **isn't your team's color** (turn ends; if it's the assassin, you lose),
- you've used `guesses_allowed` guesses (extra guesses are ignored, not penalized),
- your list runs out,
- the game ends.

Guess matching ignores case. You don't find out the result of a guess partway
through a turn. You'll see it in `history` on your next move.

## History

`history` lists every completed turn, oldest first, for **both** teams.

| field | meaning |
|---|---|
| `team` | the team that played the turn |
| `clue` | `{word, count}`, or `null` if the spymaster forfeited |
| `guesses` | each guess that was applied, with what it turned out to be |
| `end` | why the turn ended: `wrong_card`, `out_of_guesses`, `stopped`, `game_over`, `spymaster_forfeit`, `operative_forfeit` |

## Errors and timeouts

Nothing a bot does can crash the match. The worst outcome is losing your turn.
Every failure is written to the match log along with your stderr.

| What happened | Result |
|---|---|
| **Spymaster** times out, crashes (non-zero exit), sends invalid JSON, or gives an illegal clue | Turn **forfeited**. No clue, no guesses, play passes to the other team. History shows `"clue": null, "end": "spymaster_forfeit"`. |
| **Operative** times out, crashes, sends invalid JSON, or sends an empty guess list | Turn **forfeited**. No guesses are made. `"end": "operative_forfeit"` |
| **Operative** guesses a word that isn't on the board, is already revealed, or is repeated | Guesses before it **still count**. The bad guess and everything after it are dropped, and the turn ends with `"end": "operative_forfeit"`. |

If a game reaches **50 turns** without a winner, it's a **draw if no cards were revealed at all**. Otherwise the team with **more forfeits loses**. If both teams have the same number of forfeits, the team with **fewer of its own cards left** wins, and if that's also equal, it's a draw.

**There's no way to skip a turn.** Your operative must guess at least once every turn. Every forfeit
counts against you if the game reaches the turn limit, so a bot that stalls is very likely to lose.
Weak or vague clues are allowed. They just make your team slower.

## Fair play

Breaking either rule means **disqualification**.

1. **Clues must be real words, and must not hide game information.** Use ordinary English words you'd
   find in a dictionary. A weak or unhelpful clue is allowed. What isn't allowed is using a clue to pass
   information other than its meaning: made-up words, codes, or words chosen for their spelling, length,
   or position on the board (e.g. "a clue starting with `c` means guess the third card").
2. **Your spymaster and operative must not communicate during a game.** They may share code and data you
   prepare in advance, such as a word list or a model. During a game, though, the only thing the operative
   may learn from the spymaster is the clue itself. That means no writing files for the other bot to read,
   no sockets or shared servers that pass game information between them, and no hidden messages in clues.

**Ollama, outside APIs, and local model servers are allowed.** Your spymaster and operative may both
call the same model or service. The rule is only that no game information may pass between them through it:
don't store the spymaster's board or reasoning anywhere the operative can read it, and don't keep
per-game state on a server that both bots share.

If you aren't sure whether something is allowed, ask the professor before the tournament.

## Test your bot

You **don't** need Node, pnpm, or the web frontend. You need:

- **Go 1.24 or newer** (https://go.dev/dl/), to run matches
- whatever your bot needs, e.g. **Python 3**

Put your bot in `students/<your team>/` first (see [Submitting your team](#submitting-your-team)).
These examples use a team called `my_team`.

**1. Run each bot by hand** with a sample request. This shows your stderr debug output too:

```bash
cd students/my_team
python3 spymaster.py < ../../examples/sample_spymaster_request.json
python3 operative.py < ../../examples/sample_operative_request.json
cd ../..
```

Check that stdout is a single JSON object, like `{"clue": "metal", "count": 2}` or `{"guesses": ["mercury"]}`.

**2. Play a match** against one of the example teams, from the root of the repo:

```bash
go run ./cmd/match my_team test_student
```

It prints the winner and any forfeits. If your folder is set up wrong, it stops right away and says what's
missing. To see why a move failed during the game, look for `"type":"forfeit"` lines in the log file it
names, which include your stderr. To watch the game step by step, run `go run ./cmd/replay <log file>`.

**3. Run a mini tournament** against every team in `students/`:

```bash
go run ./cmd/tournament
```

**On Windows:**

- In step 1, `python3` often doesn't work. Use `python` or `py` instead. Matches pick a working Python
  automatically for teams without a `team.json`.
- If you write a `team.json` for Python, use `"python spymaster.py"`, or whatever works on the professor's
  machine. If you're unsure, leave `team.json` out.
- Paths in `team.json` can't contain spaces, even in quotes.

## Tips

- Python's interpreter startup takes about 50 ms. Loading a large model on every move could easily exceed 10 s, so precompute what you can and load only what you need.
- If you call an outside API, set your own timeout below 10 s and fall back to a safe answer, such as a count-1 clue or a single guess you're most confident in.

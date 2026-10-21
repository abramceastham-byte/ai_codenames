# Codenames Bot Guide

You'll build **two programs**: a **spymaster** that gives clues and an
**operative** that guesses. They can be written in any language and can use
any approach: your own code, a local model through Ollama, or an outside API.
The only requirement is that they follow the protocol below.

**Start from the examples** in [`examples/python/`](examples/python): `spymaster.py`
and `operative.py` follow the protocol and play legal (random) moves.

The formal definition is [`botproto/schema.json`](botproto/schema.json). If this
guide and the schema ever disagree, the schema is correct.

## How your bot is run

For **every move**, the engine:

1. starts your program (a new process each time),
2. writes **one JSON object** to its **stdin**, then closes stdin,
3. reads **one JSON object** from its **stdout**,
4. waits for the program to exit.

- You have **10 seconds** per move, counted from start to exit. The professor may change this for the tournament.
- Write **only** the JSON response to stdout. Send debug output to **stderr**. The engine saves stderr to the match log, which helps with debugging.
- Your program is stateless between moves. Everything you need is in the request, including the full history of the game.
- Responses larger than 64 KB are rejected.

## The board

There are 25 cards, listed row by row (index 0 is top-left, index 24 is bottom-right).
Every card has a hidden type: `red`, `blue`, `bystander`, or `assassin`.
The team that starts has 9 cards, the other team has 8, and there are 7 bystanders and 1 assassin.

A team wins by revealing all of its own cards. A team **loses immediately** if it reveals the assassin.

## Spymaster

### Request

The spymaster sees the type of **every** card.

```json
{
  "protocol_version": 1,
  "role": "spymaster",
  "team": "red",
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

## Operative

### Request

The operative sees the same board, but **`type` is missing on unrevealed cards**.
It also gets the clue for this turn and how many guesses it may make (**count + 1**).

```json
{
  "protocol_version": 1,
  "role": "operative",
  "team": "red",
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

To pass without guessing anything, send an empty list:

```json
{ "guesses": [] }
```

### How guesses are applied

The engine reveals your guesses one at a time, in order. It stops at the first of these:

- a guess reveals a card that **isn't your team's color** (turn ends; if it's the assassin, you lose),
- you've used `guesses_allowed` guesses (extra guesses are ignored),
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
| **Operative** times out, crashes, or sends invalid JSON | Turn **forfeited**. No guesses are made. `"end": "operative_forfeit"` |
| **Operative** guesses a word that isn't on the board, is already revealed, or is repeated | Guesses before it **still count**. The bad guess and everything after it are dropped, and the turn ends with `"end": "operative_forfeit"`. |

If a game reaches **50 turns** without a winner, the team with **fewer of its own cards left** wins. If both have the same number left, it's a draw.

## Tips

- Test your bot by hand before the tournament: save a request to a file and run `python3 my_spymaster.py < request.json`
- Python's interpreter startup takes about 50 ms. Loading a large model on every move could easily exceed 10 s, so cache or precompute where you can, or keep a separate server running that your bot talks to.
- If you call an outside API, set your own timeout below 10 s and fall back to a safe answer, such as a count-1 clue or an empty guess list.

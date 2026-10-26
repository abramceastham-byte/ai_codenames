#!/usr/bin/env python3
"""Example LLM spymaster: asks a model on Ollama for a clue.

The model's answer is never trusted: the clue is checked against the rules
and the count only includes words that really are ours. If the model is slow,
down, or gives an illegal clue, a safe count-1 clue is sent instead.
See BOT_GUIDE.md for the protocol. Debug output goes to stderr.
"""
import json
import random
import re
import sys

import llm

FALLBACK_CLUES = [
    "animal", "bright", "city", "cold", "country", "dark", "fast", "food",
    "game", "history", "house", "light", "loud", "metal", "money", "music",
    "nature", "ocean", "old", "power", "quiet", "science", "sharp", "small",
]


def is_legal(clue, board):
    """One word of a-z, 1-30 letters, not overlapping any unrevealed board word."""
    if not re.fullmatch(r"[a-z]{1,30}", clue):
        return False
    for card in board:
        word = card["word"].lower()
        if not card["revealed"] and (clue in word or word in clue):
            return False
    return True


def main():
    request = json.load(sys.stdin)
    board = request["board"]
    team = request["team"]
    unrevealed = [c for c in board if not c["revealed"]]
    mine = [c["word"] for c in unrevealed if c["type"] == team]
    assassin = [c["word"] for c in unrevealed if c["type"] == "assassin"]
    avoid = [c["word"] for c in unrevealed if c["type"] not in (team, "assassin")]

    prompt = f"""You are the spymaster in the board game Codenames.
Your team's words: {", ".join(mine)}
Words to avoid: {", ".join(avoid)}
NEVER hint at this word (instant loss): {", ".join(assassin)}

Give ONE ordinary English word as a clue that links 2 or 3 of your team's words,
and is not related to the words to avoid. The clue must not be, contain, or be
contained in any of the words above. Reply with JSON only:
{{"clue": "<one word>", "targets": ["<your word>", "<your word>"]}}"""

    # Small models often break the rules, so tell it what was wrong and retry
    # (as long as there's time left; see LLM_TIMEOUT in llm.py).
    for _ in range(3):
        try:
            answer = llm.ask_json(prompt)
        except Exception as e:  # timeout, server down, bad JSON, ...
            print(f"model failed ({e!r})", file=sys.stderr)
            break
        print(f"model said: {answer}", file=sys.stderr)
        clue = str(answer.get("clue", "")).strip().lower()
        targets = {str(t).lower() for t in answer.get("targets", [])} & set(mine)
        if not is_legal(clue, board):
            prompt += f'\n\n"{clue}" is not allowed: it must be one word of letters a-z that does not overlap any board word. Try a different clue.'
        elif not targets:
            prompt += f"\n\nYour targets must be taken from your team's words: {', '.join(mine)}. Try again."
        else:
            json.dump({"clue": clue, "count": min(len(targets), 9)}, sys.stdout)
            return

    print("no usable clue from the model, falling back", file=sys.stderr)
    clue = random.choice([w for w in FALLBACK_CLUES if is_legal(w, board)])
    json.dump({"clue": clue, "count": 1}, sys.stdout)


if __name__ == "__main__":
    main()

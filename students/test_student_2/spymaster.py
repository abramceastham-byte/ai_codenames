#!/usr/bin/env python3
"""test_student_2 spymaster: goes for big clues.

For every tag, it predicts exactly which words our operative will guess
(we share tags.py) and picks the clue with the most expected hits. It's
happy to take a clue that also touches bystanders or the other team, as
long as our operative won't reach the assassin.
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from tags import TAGS, guess_order  # noqa: E402


def is_legal(clue, board):
    if not clue.isascii() or not clue.isalpha() or not 1 <= len(clue) <= 30:
        return False
    for card in board:
        word = card["word"].lower()
        if not card["revealed"] and (clue in word or word in clue):
            return False
    return True


def evaluate(order, types, team):
    """Plays out our operative's guesses for one clue. Returns
    (count, expected value), or None if the assassin could be reached."""
    hits = 0
    for w in order:
        if types[w] == team:
            hits += 1
            continue
        if types[w] == "assassin" and not hits:
            return None
        # Count = hits, so the operative stops right before the first wrong
        # card and never reaches it. Still, an enemy or the assassin right
        # after our words costs a little, in case the order surprises us.
        penalty = {"bystander": 0.0, "assassin": 0.5}.get(types[w], 0.3)
        return (hits, hits - penalty) if hits else None
    return (hits, hits) if hits else None


def main():
    req = json.load(sys.stdin)
    board, team = req["board"], req["team"]
    types = {c["word"]: c["type"] for c in board if not c["revealed"]}

    best = None  # (value, count, clue)
    for clues, _ in TAGS:
        clue = next((c for c in clues if is_legal(c, board)), None)
        if clue is None:
            continue
        result = evaluate(guess_order(clue, types), types, team)
        if result is None:
            continue
        count, value = result
        # Riskier style: a bonus for bigger clues.
        value += 0.25 * (count - 1)
        if best is None or value > best[0]:
            best = (value, count, clue)

    if best is None:
        # No tag works safely: give the first legal tag that doesn't lead to
        # the assassin first, for 1. Our operative will still guess something.
        for clues, _ in TAGS:
            clue = next((c for c in clues if is_legal(c, board)), None)
            order = guess_order(clue, types) if clue else []
            if clue and (not order or types[order[0]] != "assassin"):
                best = (0, 1, clue)
                break

    value, count, clue = best
    print(f"clue {clue} {count} (value {value:.2f})", file=sys.stderr)
    json.dump({"clue": clue, "count": count}, sys.stdout)


if __name__ == "__main__":
    main()

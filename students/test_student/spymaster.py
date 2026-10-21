#!/usr/bin/env python3
"""test_student spymaster: picks the clue that covers the most of our
unrevealed words without touching the assassin or the other team's words."""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from themes import CANDIDATES  # noqa: E402


def is_legal(clue, board):
    if not clue.isascii() or not clue.isalpha() or not 1 <= len(clue) <= 30:
        return False
    for card in board:
        word = card["word"].lower()
        if not card["revealed"] and (clue in word or word in clue):
            return False
    return True


def main():
    req = json.load(sys.stdin)
    board, team = req["board"], req["team"]
    unrevealed = {c["word"]: c["type"] for c in board if not c["revealed"]}

    options = []  # (risk, -score, clue, own words)
    for clues, words in CANDIDATES:
        on_board = {w for w in words if w in unrevealed}
        types = [unrevealed[w] for w in on_board]
        own = sorted(w for w in on_board if unrevealed[w] == team)
        clue = next((c for c in clues if is_legal(c, board)), None)
        if not own or clue is None:
            continue
        # The operative can't tell our words from others within a clue, so
        # prefer clues that point only at our words. Bystanders are a small
        # risk, the other team's words a bigger one, the assassin fatal.
        enemy = sum(1 for t in types if t not in (team, "bystander", "assassin"))
        risk = 2 if "assassin" in types else (1 if enemy else 0)
        score = len(own) - 0.6 * types.count("bystander")
        options.append((risk, -score, clue, own))

    risk, neg_score, clue, own = min(options)
    print(f"clue {clue} for {own} (risk {risk}, score {-neg_score:.1f})", file=sys.stderr)
    json.dump({"clue": clue, "count": len(own)}, sys.stdout)


if __name__ == "__main__":
    main()

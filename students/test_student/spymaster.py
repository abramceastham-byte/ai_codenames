#!/usr/bin/env python3
"""test_student spymaster: picks the theme that covers the most of our
unrevealed words without touching the assassin or the other team's words."""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from themes import THEMES  # noqa: E402

SAFE_FALLBACKS = ["something", "anything", "whatever", "random", "hmm"]


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

    best = None  # (score, clue, own words)
    for clues, words in THEMES:
        on_board = {w for w in words if w in unrevealed}
        types = [unrevealed[w] for w in on_board]
        own = [w for w in on_board if unrevealed[w] == team]
        if not own or "assassin" in types:
            continue
        enemy = sum(1 for t in types if t not in (team, "bystander", "assassin"))
        bystanders = types.count("bystander")
        if enemy:
            # The operative can't tell our words from theirs within a theme.
            continue
        clue = next((c for c in clues if is_legal(c, board)), None)
        if clue is None:
            continue
        # Bystanders aren't fatal but can end the turn early.
        score = len(own) - 0.6 * bystanders
        if best is None or score > best[0]:
            best = (score, clue, own)

    if best and best[0] > 0:
        _, clue, own = best
        print(f"clue {clue} for {own}", file=sys.stderr)
        json.dump({"clue": clue, "count": len(own)}, sys.stdout)
        return

    # Nothing fits: give a clue our operative won't recognize, so it passes
    # instead of guessing blindly.
    clue = next(c for c in SAFE_FALLBACKS if is_legal(c, board))
    print(f"no safe theme, fallback {clue}", file=sys.stderr)
    json.dump({"clue": clue, "count": 1}, sys.stdout)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""test_student_2 operative: guesses the clue's words most-certain first,
exactly as many as the count. Our spymaster predicts this order, so the count
already stops right before the first card that isn't ours. That's why we
skip the bonus guess: the next word in the order is usually a wrong card."""
import json
import os
import random
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from tags import guess_order  # noqa: E402


def main():
    req = json.load(sys.stdin)
    clue = req["clue"]
    unrevealed = {c["word"] for c in req["board"] if not c["revealed"]}

    guesses = guess_order(clue["word"], unrevealed)[: clue["count"]]

    if not guesses:
        # At least one guess is required.
        guesses = [random.choice(sorted(unrevealed))]

    print(f"clue {clue['word']} {clue['count']} -> {guesses}", file=sys.stderr)
    json.dump({"guesses": guesses}, sys.stdout)


if __name__ == "__main__":
    main()

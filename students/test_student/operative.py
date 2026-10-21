#!/usr/bin/env python3
"""test_student operative: guesses unrevealed board words linked to the
clue, most specific first. Always makes at least one guess."""
import json
import os
import random
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from themes import theme_count, theme_for_clue  # noqa: E402


def main():
    req = json.load(sys.stdin)
    clue = req["clue"]
    unrevealed = [c["word"] for c in req["board"] if not c["revealed"]]

    matches = [w for w in unrevealed if w in theme_for_clue(clue["word"])]
    matches.sort(key=theme_count)
    guesses = matches[: clue["count"]]
    if not guesses:
        # At least one guess is required; this shouldn't happen with our own
        # spymaster, but don't forfeit if it does.
        guesses = [random.choice(unrevealed)]

    print(f"clue {clue['word']} {clue['count']} -> {guesses}", file=sys.stderr)
    json.dump({"guesses": guesses}, sys.stdout)


if __name__ == "__main__":
    main()

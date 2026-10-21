#!/usr/bin/env python3
"""test_student operative: guesses unrevealed board words in the clue's
theme, most specific first, and stops instead of guessing at random."""
import json
import os
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

    print(f"clue {clue['word']} {clue['count']} -> {guesses}", file=sys.stderr)
    json.dump({"guesses": guesses}, sys.stdout)


if __name__ == "__main__":
    main()

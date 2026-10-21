#!/usr/bin/env python3
"""Example spymaster: gives a random legal clue with count 1.

Reads one request (JSON) from stdin, writes one response (JSON) to stdout.
See BOT_GUIDE.md for the protocol. Debug output goes to stderr.
"""
import json
import random
import sys

CLUE_WORDS = [
    "animal", "bright", "city", "cold", "country", "dark", "fast", "food",
    "game", "history", "house", "light", "loud", "metal", "money", "music",
    "nature", "ocean", "old", "power", "quiet", "science", "sharp", "small",
    "sport", "story", "travel", "water", "weather", "work",
]


def is_legal(clue, board):
    """A clue can't contain, or be contained in, any unrevealed board word."""
    for card in board:
        word = card["word"].lower()
        if not card["revealed"] and (clue in word or word in clue):
            return False
    return True


def main():
    request = json.load(sys.stdin)
    board = request["board"]

    my_words = [c["word"] for c in board if c["type"] == request["team"] and not c["revealed"]]
    print(f"{len(my_words)} words left to find: {my_words}", file=sys.stderr)

    legal = [w for w in CLUE_WORDS if is_legal(w, board)]
    clue = random.choice(legal)

    json.dump({"clue": clue, "count": 1}, sys.stdout)


if __name__ == "__main__":
    main()

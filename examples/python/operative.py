#!/usr/bin/env python3
"""Example operative: guesses random unrevealed words.

Reads one request (JSON) from stdin, writes one response (JSON) to stdout.
See BOT_GUIDE.md for the protocol. Debug output goes to stderr.
"""
import json
import random
import sys


def main():
    request = json.load(sys.stdin)
    clue = request["clue"]
    print(f"Clue is {clue['word']} {clue['count']}", file=sys.stderr)

    unrevealed = [c["word"] for c in request["board"] if not c["revealed"]]
    guesses = random.sample(unrevealed, min(clue["count"], len(unrevealed)))

    json.dump({"guesses": guesses}, sys.stdout)


if __name__ == "__main__":
    main()

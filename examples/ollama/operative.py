#!/usr/bin/env python3
"""Example LLM operative: asks a model on Ollama which words match the clue.

Only real, unrevealed board words are kept, and at most `count` of them (the
bonus guess is skipped to stay safe). If the model gives nothing usable, one
random unrevealed word is guessed so the turn isn't forfeited.
See BOT_GUIDE.md for the protocol. Debug output goes to stderr.
"""
import json
import random
import sys

import llm


def main():
    request = json.load(sys.stdin)
    clue = request["clue"]
    unrevealed = [c["word"] for c in request["board"] if not c["revealed"]]

    prompt = f"""You are the operative in the board game Codenames.
Your spymaster's clue is "{clue["word"]}" for {clue["count"]} word(s).
Words on the board: {", ".join(unrevealed)}

Pick the {clue["count"]} board word(s) most related to the clue, most confident first.
Use the words exactly as written. Reply with JSON only:
{{"guesses": ["<board word>", ...]}}"""

    guesses = []
    try:
        answer = llm.ask_json(prompt)
        print(f"model said: {answer}", file=sys.stderr)
        for g in answer.get("guesses", []):
            g = str(g).strip().lower()
            if g in unrevealed and g not in guesses:
                guesses.append(g)
    except Exception as e:  # timeout, server down, bad JSON, ...
        print(f"model failed ({e!r})", file=sys.stderr)

    guesses = guesses[:clue["count"]]
    if not guesses:
        print("nothing usable, guessing at random", file=sys.stderr)
        guesses = [random.choice(unrevealed)]
    json.dump({"guesses": guesses}, sys.stdout)


if __name__ == "__main__":
    main()

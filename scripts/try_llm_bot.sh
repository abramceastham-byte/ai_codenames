#!/usr/bin/env bash
# Plays the example Ollama bot (examples/ollama) against another team, in Docker,
# with the bundled Ollama server. The model is pulled the first time.
#
#   ./scripts/try_llm_bot.sh [opponent] [model]     # defaults: test_student gemma3:1b
#
# The bot is copied to students/llm_example first, so it also takes part in
# later tournaments; delete that folder if you don't want it there.
set -euo pipefail
cd "$(dirname "$0")/.."
opponent=${1:-test_student}
export OLLAMA_MODEL=${2:-${OLLAMA_MODEL:-gemma3:1b}}
# Without a GPU a move can take a while, so give each one 30 s, and tell the
# bot to give up on the model (and play a safe move) a bit before that.
export LLM_TIMEOUT=${LLM_TIMEOUT:-25}

mkdir -p students/llm_example
cp examples/ollama/*.py students/llm_example/
docker compose --profile ollama run --rm tournament match --move_timeout 30s llm_example "$opponent"

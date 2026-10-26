#!/usr/bin/env bash
# Runs the ollama CLI inside the bundled Ollama container (docker-compose.yml),
# starting the container first if it isn't running.
#
#   ./scripts/ollama.sh list                 # models already pulled
#   ./scripts/ollama.sh pull gemma3:1b       # pull one ahead of time
#   ./scripts/ollama.sh run gemma3:1b "hi"   # talk to it directly
set -euo pipefail
cd "$(dirname "$0")/.."
docker compose --profile ollama up -d --wait ollama >&2
exec docker compose --profile ollama exec ollama ollama "$@"

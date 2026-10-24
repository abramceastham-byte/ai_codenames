#!/usr/bin/env bash
# Lists the models already pulled into OLLAMA_MODELS_DIR (default ~/.ollama),
# i.e. the values you can give OLLAMA_MODEL.
set -euo pipefail
dir="${OLLAMA_MODELS_DIR:-$HOME/.ollama}/models/manifests"
if [ ! -d "$dir" ]; then
  echo "no models found under $dir (set OLLAMA_MODELS_DIR?)" >&2
  exit 1
fi
find "$dir" -type f | sed "s#^$dir/[^/]*/##" | sed -E 's#^library/##; s#/([^/]+)$#:\1#' | sort

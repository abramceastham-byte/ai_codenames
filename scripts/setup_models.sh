#!/usr/bin/env bash
# Downloads pretrained GloVe + ConceptNet Numberbatch vectors and converts
# them to the word2vec binary format expected at data/glove.bin and
# data/conceptnet.bin (see w2v/README.md and cmd/ai-server). Safe to re-run:
# skips any file that's already present.
#
# Override the source/size via env vars, e.g. to use a larger GloVe model:
#   GLOVE_URL=https://nlp.stanford.edu/data/glove.840B.300d.zip \
#   GLOVE_TXT_NAME=glove.840B.300d.txt \
#   scripts/setup_models.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_DIR="${DATA_DIR:-$REPO_ROOT/data}"

GLOVE_URL="${GLOVE_URL:-https://nlp.stanford.edu/data/glove.6B.zip}"
GLOVE_TXT_NAME="${GLOVE_TXT_NAME:-glove.6B.300d.txt}"
NUMBERBATCH_URL="${NUMBERBATCH_URL:-https://conceptnet.s3.amazonaws.com/downloads/2019/numberbatch/numberbatch-en-19.08.txt.gz}"

mkdir -p "$DATA_DIR"

need_glove=1
[ -f "$DATA_DIR/glove.bin" ] && need_glove=0
need_conceptnet=1
[ -f "$DATA_DIR/conceptnet.bin" ] && need_conceptnet=0

if [ "$need_glove" -eq 0 ] && [ "$need_conceptnet" -eq 0 ]; then
  echo "data/glove.bin and data/conceptnet.bin already exist, nothing to do"
  exit 0
fi

command -v curl >/dev/null 2>&1 || {
  echo "error: curl is required but not found on PATH" >&2
  exit 1
}
command -v unzip >/dev/null 2>&1 || {
  echo "error: unzip is required but not found on PATH" >&2
  exit 1
}

PYTHON=""
for candidate in python3 python; do
  # command -v alone isn't enough: on Windows, "python3"/"python" can resolve
  # to a Microsoft Store stub that's present on PATH but exits nonzero (and
  # prints an unhelpful install prompt) when actually invoked.
  if command -v "$candidate" >/dev/null 2>&1 && "$candidate" --version >/dev/null 2>&1; then
    PYTHON="$candidate"
    break
  fi
done
if [ -z "$PYTHON" ]; then
  echo "error: python3 is required to run scripts/convert_to_w2v_binary.py." >&2
  echo "       It's not otherwise a project dependency (Go + Node/pnpm cover" >&2
  echo "       everything else) - install it from https://www.python.org/downloads/" >&2
  echo "       and re-run this script." >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ "$need_glove" -eq 0 ]; then
  echo "data/glove.bin already exists, skipping GloVe download"
else
  echo "Downloading GloVe vectors from $GLOVE_URL ..."
  curl -fL "$GLOVE_URL" -o "$tmp/glove.zip"
  echo "Extracting $GLOVE_TXT_NAME ..."
  unzip -p "$tmp/glove.zip" "$GLOVE_TXT_NAME" >"$tmp/glove.txt"
  echo "Converting to word2vec binary format (this can take a few minutes) ..."
  "$PYTHON" "$SCRIPT_DIR/convert_to_w2v_binary.py" "$tmp/glove.txt" "$DATA_DIR/glove.bin"
  rm -f "$tmp/glove.zip" "$tmp/glove.txt"
fi

if [ "$need_conceptnet" -eq 0 ]; then
  echo "data/conceptnet.bin already exists, skipping ConceptNet download"
else
  echo "Downloading ConceptNet Numberbatch vectors from $NUMBERBATCH_URL ..."
  curl -fL "$NUMBERBATCH_URL" -o "$tmp/numberbatch.txt.gz"
  echo "Converting to word2vec binary format (this can take a few minutes) ..."
  "$PYTHON" "$SCRIPT_DIR/convert_to_w2v_binary.py" "$tmp/numberbatch.txt.gz" "$DATA_DIR/conceptnet.bin" --has-header
  rm -f "$tmp/numberbatch.txt.gz"
fi

echo "Done. Models are in $DATA_DIR"

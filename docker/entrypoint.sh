#!/usr/bin/env bash
# Installs the Python packages students ask for, gets the Ollama model ready,
# then runs the requested command.
#
#   entrypoint.sh [tournament flags...]   -> tournament
#   entrypoint.sh match TEAM TEAM ...     -> match
#   entrypoint.sh replay FILE             -> replay
#
# Any students/<team>/requirements.txt is installed (all teams in one pip run,
# so conflicting pins fail early) into /deps, which is a volume, so a package
# is only downloaded again when a requirements.txt changes.
#
# If OLLAMA_MODEL is set, the model is pulled (first time only) and loaded
# before the first move, so the first bot to use it doesn't time out.
#
# The command runs as the owner of the students folder rather than root, so
# logs and results on the host belong to you.
set -euo pipefail

cmd=tournament
case "${1:-}" in
  tournament | match | replay) cmd=$1; shift ;;
esac

students="${STUDENTS_DIR:-/app/students}"
logs="${LOGS_DIR:-/app/logs}"

if [ "$cmd" != replay ]; then
  mapfile -t reqs < <(find "$students" -mindepth 2 -maxdepth 2 -name requirements.txt | sort)
  if [ "${#reqs[@]}" -gt 0 ]; then
    sum=$(cat "${reqs[@]}" | sha256sum | cut -d' ' -f1)
    if [ "$(cat /deps/.sum 2>/dev/null || true)" != "$sum" ]; then
      args=()
      for r in "${reqs[@]}"; do args+=(-r "$r"); done
      echo "Installing Python packages for: $(printf '%s ' "${reqs[@]#"$students"/}")" >&2
      pip install --quiet --target /deps --upgrade "${args[@]}" >&2 \
        || { echo "pip install failed; fix the requirements.txt files above and rerun" >&2; exit 1; }
      echo "$sum" > /deps/.sum
    fi
  fi

  if [ -n "${OLLAMA_MODEL:-}" ]; then
    model="{\"model\":\"$OLLAMA_MODEL\"}"
    curl -sf -m 5 "$OLLAMA_HOST/api/version" > /dev/null \
      || { echo "Can't reach Ollama at $OLLAMA_HOST. Add --profile ollama, or set OLLAMA_HOST." >&2; exit 1; }
    if ! curl -sf -d "$model" "$OLLAMA_HOST/api/show" > /dev/null; then
      echo "Pulling $OLLAMA_MODEL (first time only)..." >&2
      curl -sf -d "{\"model\":\"$OLLAMA_MODEL\",\"stream\":false}" "$OLLAMA_HOST/api/pull" > /dev/null \
        || { echo "Couldn't pull $OLLAMA_MODEL; check the name at https://ollama.com/library" >&2; exit 1; }
    fi
    echo "Loading $OLLAMA_MODEL..." >&2
    curl -sf -d "{\"model\":\"$OLLAMA_MODEL\",\"keep_alive\":\"1h\"}" "$OLLAMA_HOST/api/generate" > /dev/null \
      || { echo "Ollama couldn't load $OLLAMA_MODEL" >&2; exit 1; }
  fi
fi

uid=$(stat -c %u "$students")
gid=$(stat -c %g "$students")
if [ "$(id -u)" = 0 ] && [ "$uid" != 0 ]; then
  mkdir -p "$logs"
  # Also hands back anything an older, root-run version of this image wrote.
  find "$logs" -uid 0 -exec chown "$uid:$gid" {} +
  export HOME=/tmp
  exec setpriv --reuid="$uid" --regid="$gid" --clear-groups -- "$cmd" "$@"
fi
exec "$cmd" "$@"

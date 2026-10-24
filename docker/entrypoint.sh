#!/usr/bin/env bash
# Installs the Python packages students ask for, then runs the requested command.
#
#   entrypoint.sh [tournament flags...]   -> tournament
#   entrypoint.sh match TEAM TEAM ...     -> match
#   entrypoint.sh replay FILE             -> replay
#
# Any students/<team>/requirements.txt is installed (all teams in one pip run,
# so conflicting pins fail early) into /deps, which is a volume, so a package
# is only downloaded again when a requirements.txt changes.
set -euo pipefail

cmd=tournament
case "${1:-}" in
  tournament | match | replay) cmd=$1; shift ;;
esac

students="${STUDENTS_DIR:-/app/students}"
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
fi

exec "$cmd" "$@"

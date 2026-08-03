#!/usr/bin/env python3
"""Merge logs/ai_reasoning.jsonl (structured final decisions) with the raw
per-attempt model responses from the ai-server log into one chronological
JSONL, for reviewing AI behavior during research trials.

There's no shared request ID between the two log sources, so correlation is
done by timestamp: each decision claims the raw events between the *previous*
decision's timestamp and its own (a raw response always happens before the
decision that used it gets logged, never after), with --window-seconds only
as a fallback cap for the first entry or an unusually large gap. A symmetric
window was tried first and abandoned - back-to-back turns can be only a few
seconds apart, which let an earlier decision steal a later one's response.
This still assumes one game's turns aren't interleaved in wall-clock time
with another game's (the raw log lines don't carry a game_id to disambiguate
that) - fine for one game at a time, but multiple concurrent games could
misattribute a raw attempt to the wrong game.

ai_reasoning.jsonl timestamps are UTC; the ai-server log's are in the local
system timezone, so this converts the latter to UTC using the machine's
current local offset before comparing. That's exact for logs generated the
same day the script runs; if your trial data spans a DST transition,
attempts near the boundary may drift - spot check those manually.

Anything that can't be matched to a nearby decision is still included,
tagged as "unmatched_raw_event", so nothing is silently dropped.

Alongside the JSONL, this also writes a plain-text transcript (same path,
.txt extension) meant for actually reading through - one block per
clue/guess in chronological order, with the final result and every raw
attempt that led to it.

Usage:
    python3 scripts/merge_reasoning_data.py \
        --reasoning-log logs/ai_reasoning.jsonl \
        --server-log logs/ai-server.log \
        --output logs/merged_reasoning.jsonl

    # Just the last 3 games instead of the whole accumulated log:
    python3 scripts/merge_reasoning_data.py --games 3

    # Keep re-running while a game is in progress, so an editor with
    # logs/merged_reasoning.txt open picks up new turns as they happen:
    python3 scripts/merge_reasoning_data.py --games 1 --watch
"""
import argparse
import json
import re
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path

SERVER_LOG_TS_FMT = "%Y/%m/%d %H:%M:%S"

# Matches both:
#   [LLM Spymaster] attempt=1, raw response: "..."
#   [LLM Operative] clue="word", attempt=1, raw response: "..."
ATTEMPT_RE = re.compile(
    r'^(?P<ts>\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) '
    r'\[LLM (?P<role>Spymaster|Operative)\] '
    r'(?:clue=(?P<clue>".*?"), )?'
    r'attempt=(?P<attempt>\d+), raw response: (?P<response>".*")$'
)
REJECTED_RE = re.compile(
    r'^(?P<ts>\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) '
    r'\[LLM Spymaster\] rejected attempt=(?P<attempt>\d+): (?P<reason>.*)$'
)
FAILED_RE = re.compile(
    r'^(?P<ts>\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) '
    r'\[ERROR\] AI failed to make a clue: (?P<error>.*)$'
)

# The local offset in effect right now on this machine, applied to every
# server-log timestamp. See the DST caveat in the module docstring.
LOCAL_TZINFO = datetime.now().astimezone().tzinfo


def parse_go_quoted(s):
    """Go's %q output is close enough to JSON string syntax to decode directly."""
    if s is None:
        return None
    try:
        return json.loads(s)
    except json.JSONDecodeError:
        return s


def local_to_utc(naive_local_ts):
    return naive_local_ts.replace(tzinfo=LOCAL_TZINFO).astimezone(timezone.utc)


def parse_server_log(path):
    """Returns a chronological list of raw attempt/rejection/failure events,
    each with a "ts" key holding a UTC-aware datetime."""
    events = []
    try:
        f = open(path, "r", encoding="utf-8", errors="replace")
    except FileNotFoundError:
        # The ai-server logs to stdout by default; this file only exists if
        # someone redirected it themselves. Raw attempts are a bonus
        # correlation on top of ai_reasoning.jsonl, not a requirement.
        print(f"WARNING: {path} not found, continuing without raw attempt data")
        return events
    with f:
        for line in f:
            line = line.rstrip("\n")

            m = ATTEMPT_RE.match(line)
            if m:
                events.append({
                    "ts": local_to_utc(datetime.strptime(m.group("ts"), SERVER_LOG_TS_FMT)),
                    "type": "attempt",
                    "role": m.group("role"),
                    "attempt": int(m.group("attempt")),
                    "clue": parse_go_quoted(m.group("clue")),
                    "raw_response": parse_go_quoted(m.group("response")),
                })
                continue

            m = REJECTED_RE.match(line)
            if m:
                events.append({
                    "ts": local_to_utc(datetime.strptime(m.group("ts"), SERVER_LOG_TS_FMT)),
                    "type": "rejected",
                    "attempt": int(m.group("attempt")),
                    "reason": m.group("reason"),
                })
                continue

            m = FAILED_RE.match(line)
            if m:
                events.append({
                    "ts": local_to_utc(datetime.strptime(m.group("ts"), SERVER_LOG_TS_FMT)),
                    "type": "failed",
                    "error": m.group("error"),
                })

    events.sort(key=lambda e: e["ts"])
    return events


def parse_reasoning_log(path):
    entries = []
    with open(path, "r", encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                # A process killed mid-write (e.g. a server restart) can
                # leave a few stray bytes glued onto the front of the next
                # line, since the file is append-only and nothing rewinds
                # past a partial write. That garbage is always a prefix
                # before the real '{', so recover by reparsing from there
                # rather than losing the entry outright.
                brace = line.find("{")
                if brace <= 0:
                    print(f"WARNING: skipping unparseable line {lineno} in {path}: {line[:80]!r}")
                    continue
                try:
                    entry = json.loads(line[brace:])
                    print(f"WARNING: recovered line {lineno} in {path} after stripping leading {line[:brace]!r}")
                except json.JSONDecodeError:
                    print(f"WARNING: skipping unparseable line {lineno} in {path}: {line[:80]!r}")
                    continue
            raw_ts = entry.get("timestamp", "")
            try:
                entry["_ts"] = datetime.fromisoformat(raw_ts.replace("Z", "+00:00"))
            except ValueError:
                entry["_ts"] = None
            entries.append(entry)
    entries.sort(key=lambda e: e["_ts"] or datetime.min.replace(tzinfo=timezone.utc))
    return entries


def event_to_json(ev):
    return {k: (v.isoformat() if k == "ts" else v) for k, v in ev.items()}


def format_attempt_line(ev):
    if ev["type"] == "attempt":
        clue_part = f'clue={ev["clue"]!r} ' if ev.get("clue") else ""
        return f'  [attempt {ev["attempt"]}] {clue_part}raw response: {ev["raw_response"]}'
    if ev["type"] == "rejected":
        return f'  [attempt {ev["attempt"]}] REJECTED: {ev["reason"]}'
    if ev["type"] == "failed":
        return f'  [failed] {ev["error"]}'
    return f"  [unknown event] {ev}"


def format_text_entry(m):
    lines = [
        "=" * 80,
        f'Game: {m["game_id"]} | Round {m["round"]} | {m["team"]} {m["role"]} ({m["backend"]})',
        f'Time: {m["timestamp"]}',
        f'Action: {m["action"]}',
        "-" * 80,
    ]
    if m["final_detail"]:
        lines.append(f'Result: {m["final_detail"]}')
    if m["final_reasoning"]:
        lines.append(f'Reasoning: {m["final_reasoning"]}')
    if m["error"]:
        lines.append(f'ERROR: {m["error"]}')
    if m["suspected_compound"]:
        lines.append(f'Suspected compound: {m["suspected_compound"]}')

    if m["raw_attempts"]:
        lines.append("")
        lines.append("Attempts:")
        for ev in m["raw_attempts"]:
            lines.append(format_attempt_line(ev))
    else:
        lines.append("")
        lines.append("(no raw attempts matched)")

    return "\n".join(lines)


def write_text_transcript(path, merged, unmatched):
    with open(path, "w", encoding="utf-8") as out:
        for m in merged:
            out.write(format_text_entry(m) + "\n\n")
        if unmatched:
            out.write("=" * 80 + "\n")
            out.write(f"UNMATCHED RAW EVENTS ({len(unmatched)})\n")
            out.write("no nearby decision in ai_reasoning.jsonl to attach these to\n")
            out.write("-" * 80 + "\n")
            for ev in unmatched:
                out.write(f'{ev["ts"]}\n')
                out.write(format_attempt_line(ev) + "\n\n")


def run_once(args):
    reasoning = parse_reasoning_log(args.reasoning_log)
    events = parse_server_log(args.server_log)
    window = timedelta(seconds=args.window_seconds)

    kept_games = None
    if args.games:
        seen_order = []
        for entry in reasoning:
            gid = entry.get("game_id")
            if gid not in seen_order:
                seen_order.append(gid)
        kept_games = seen_order[-args.games:]
        keep_games = set(kept_games)
        reasoning = [e for e in reasoning if e.get("game_id") in keep_games]

        # Also drop raw events far outside the kept games' time range, so
        # "unmatched" doesn't dump in unrelated history from other games.
        ts_values = [e["_ts"] for e in reasoning if e["_ts"] is not None]
        if ts_values:
            lo, hi = min(ts_values) - window, max(ts_values) + window
            events = [ev for ev in events if lo <= ev["ts"] <= hi]

    used = [False] * len(events)

    merged = []
    prev_ts = None
    for entry in reasoning:
        ts = entry["_ts"]
        attempts = []
        if ts is not None:
            # A decision's raw attempts always happen strictly before its own
            # log line (the response is generated, then logged), never after.
            # Bound the search to (previous decision's timestamp, this one's
            # timestamp] rather than a symmetric window - back-to-back turns
            # can be only a few seconds apart, and a wide symmetric window
            # lets an earlier decision steal a later one's raw response.
            # +/-2s pads for Go's whole-second log timestamps rounding
            # differently than ai_reasoning.jsonl's sub-second UTC ones.
            lo = ts - window if prev_ts is None else max(ts - window, prev_ts - timedelta(seconds=2))
            hi = ts + timedelta(seconds=2)
            for i, ev in enumerate(events):
                if not used[i] and lo <= ev["ts"] <= hi:
                    attempts.append(ev)
                    used[i] = True
            prev_ts = ts
        attempts.sort(key=lambda e: e["ts"])
        merged.append({
            "timestamp": entry.get("timestamp"),
            "game_id": entry.get("game_id"),
            "round": entry.get("round"),
            "team": entry.get("team"),
            "role": entry.get("role"),
            "backend": entry.get("backend"),
            "action": entry.get("action"),
            "final_detail": entry.get("detail"),
            "final_reasoning": entry.get("reasoning"),
            "error": entry.get("error") or None,
            "suspected_compound": entry.get("suspected_compound") or None,
            "raw_attempts": [event_to_json(ev) for ev in attempts],
        })

    unmatched = [event_to_json(ev) for i, ev in enumerate(events) if not used[i]]

    with open(args.output, "w", encoding="utf-8") as out:
        for m in merged:
            out.write(json.dumps(m, ensure_ascii=False) + "\n")
        for u in unmatched:
            out.write(json.dumps({"unmatched_raw_event": u}, ensure_ascii=False) + "\n")

    text_path = Path(args.output).with_suffix(".txt")
    write_text_transcript(text_path, merged, unmatched)

    games_note = f" [games: {', '.join(kept_games)}]" if kept_games else ""
    print(f"Wrote {len(merged)} decisions ({len(unmatched)} unmatched raw events) to {args.output} and {text_path}{games_note}")


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("--reasoning-log", default="logs/ai_reasoning.jsonl")
    ap.add_argument("--server-log", default="logs/ai-server.log")
    ap.add_argument("--output", default="logs/merged_reasoning.jsonl")
    ap.add_argument(
        "--window-seconds", type=int, default=10,
        help="max time gap (each direction) to associate a raw attempt with a decision",
    )
    ap.add_argument(
        "--games", type=int, default=None,
        help="only include the last N distinct games (by chronological order), instead of the whole log",
    )
    ap.add_argument(
        "--watch", action="store_true",
        help="re-run the merge on a timer instead of once, so an open editor "
             "showing the output picks up new turns as they happen",
    )
    ap.add_argument(
        "--watch-interval", type=float, default=5.0,
        help="seconds between re-runs when --watch is set",
    )
    args = ap.parse_args()

    if not args.watch:
        run_once(args)
        return

    print(f"Watching (every {args.watch_interval}s) - Ctrl+C to stop")
    try:
        while True:
            try:
                run_once(args)
            except FileNotFoundError as e:
                # Likely a game just started and hasn't written anything yet -
                # keep watching instead of dying.
                print(f"  ({e}, will retry)")
            time.sleep(args.watch_interval)
    except KeyboardInterrupt:
        print("\nStopped.")


if __name__ == "__main__":
    main()

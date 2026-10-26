"""Tiny Ollama client shared by spymaster.py and operative.py (standard library only).

Settings come from the environment:
  OLLAMA_HOST   server address (default http://localhost:11434)
  OLLAMA_MODEL  model name (default gemma3:1b)
  LLM_TIMEOUT   total seconds this move may spend on the model, across all calls
                (default 8, under the 10 s move limit)
"""
import json
import os
import time
import urllib.request

HOST = os.environ.get("OLLAMA_HOST") or "http://localhost:11434"
MODEL = os.environ.get("OLLAMA_MODEL") or "gemma3:1b"
DEADLINE = time.monotonic() + float(os.environ.get("LLM_TIMEOUT") or 8)


def ask_json(prompt):
    """Sends one prompt and returns the model's answer parsed as JSON.

    Raises on a timeout, a server error, or an answer that isn't JSON, so the
    caller can fall back to a safe move.
    """
    left = DEADLINE - time.monotonic()
    if left < 0.5:
        raise TimeoutError("out of time for this move")
    host = HOST if HOST.startswith("http") else "http://" + HOST
    body = json.dumps({
        "model": MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "stream": False,
        "format": "json",  # makes Ollama return valid JSON
        "options": {"temperature": 0.2},
        "keep_alive": "1h",  # keep the model loaded between moves
    }).encode()
    req = urllib.request.Request(host.rstrip("/") + "/api/chat", data=body,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=left) as resp:
        content = json.load(resp)["message"]["content"]
    return json.loads(content)

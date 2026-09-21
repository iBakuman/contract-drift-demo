#!/usr/bin/env bash
#
# Start the server, run the client against all three scenarios, stop the server.
# Everything happens in this one shell so the background server outlives the
# line that started it.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PORT=8099
WORK="$ROOT/.demo-tmp"
BIN="$WORK/server"
LOG="$WORK/server.log"

# Whatever goes wrong, say so — including what the server thought about it.
on_error() {
  echo "" >&2
  echo "demo.sh failed at line $1." >&2
  if [ -s "$LOG" ]; then
    echo "--- server log ---" >&2
    cat "$LOG" >&2
  fi
}
trap 'on_error $LINENO' ERR

for tool in go pnpm curl; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "$tool is not on PATH; the demo needs go, pnpm and curl." >&2
    exit 1
  }
done

if [ ! -d frontend/node_modules ]; then
  echo "frontend dependencies are not installed. Run:" >&2
  echo "  cd frontend && pnpm install" >&2
  exit 1
fi

mkdir -p "$WORK"
(cd backend && go build -o "$BIN" .)

# The server's own log would interleave with the demo output, so send it to a
# file and show it only when something goes wrong.
"$BIN" >"$LOG" 2>&1 &
SERVER_PID=$!

cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

ready=""
for _attempt in $(seq 1 100); do
  if curl -sf "http://localhost:$PORT/widgets?scenario=ok" >/dev/null 2>&1; then
    ready=yes
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "the server exited before it was ready:" >&2
    cat "$LOG" >&2
    exit 1
  fi
  sleep 0.1
done

if [ -z "$ready" ]; then
  echo "the server did not answer on port $PORT within 10s:" >&2
  cat "$LOG" >&2
  exit 1
fi

cd frontend && pnpm consume

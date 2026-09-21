#!/usr/bin/env bash
#
# Start the server, run the client against all three scenarios, stop the server.
# Everything happens in this one shell so the background server outlives the
# line that started it.
set -euo pipefail

cd "$(dirname "$0")/.."

PORT=8099
BIN="$(mktemp -t contract-drift-demo-server)"

if [ ! -d frontend/node_modules ]; then
  echo "frontend dependencies are not installed. Run:" >&2
  echo "  cd frontend && pnpm install" >&2
  exit 1
fi

(cd backend && go build -o "$BIN" .)

# The server's own log would interleave with the demo output, so send it to a
# file and only show it if the server fails to come up.
LOG="$(mktemp -t contract-drift-demo-log)"
"$BIN" >"$LOG" 2>&1 &
SERVER_PID=$!

cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  rm -f "$BIN" "$LOG"
}
trap cleanup EXIT

for _ in $(seq 1 100); do
  if curl -sf "http://localhost:$PORT/widgets?scenario=ok" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "the server exited before it was ready:" >&2
    cat "$LOG" >&2
    exit 1
  fi
  sleep 0.1
done

if ! curl -sf "http://localhost:$PORT/widgets?scenario=ok" >/dev/null 2>&1; then
  echo "the server did not become ready on port $PORT within 10s:" >&2
  cat "$LOG" >&2
  exit 1
fi

cd frontend && pnpm --silent consume

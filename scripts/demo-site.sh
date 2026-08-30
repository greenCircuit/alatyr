#!/bin/bash
# Build and serve the static demo site the way GitHub Pages serves it: no Go
# backend at runtime, every /api/* call resolved from the frozen snapshot in
# ui/public/demo/.
#
# Mirrors the demo-site CI job (.gitlab-ci.yml) so a local run fails on the same
# things CI fails on — including a demo scenario that stopped producing its
# finding, which is checked before the snapshot is written.
#
#   ./scripts/demo-site.sh              build, verify, serve on :4173
#   ./scripts/demo-site.sh --no-serve   build + verify only (CI-shaped)
#   ./scripts/demo-site.sh --skip-check skip the scenario assertions
#
# Env: BACKEND_PORT (default 8090, the throwaway snapshot backend)
#      PREVIEW_PORT (default 4173, the static server)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_PORT="${BACKEND_PORT:-8090}"
PREVIEW_PORT="${PREVIEW_PORT:-4173}"
BIN="$ROOT/.demo-backend"
SNAPSHOT_DIR="$ROOT/ui/public/demo"

serve=true
run_check=true
for arg in "$@"; do
  case "$arg" in
    --no-serve)   serve=false ;;
    --skip-check) run_check=false ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

backend_pid=""
cleanup() {
  [ -n "$backend_pid" ] && kill "$backend_pid" 2>/dev/null || true
  rm -f "$BIN"
}
trap cleanup EXIT

echo "==> building snapshot backend"
cd "$ROOT"
GOTOOLCHAIN=auto go build -o "$BIN" .

echo "==> starting backend on :$BACKEND_PORT (DEMO_MODE)"
DEMO_MODE=true BACKEND_PORT="$BACKEND_PORT" "$BIN" >"$ROOT/demo-backend.log" 2>&1 &
backend_pid=$!

ready=false
for _ in $(seq 1 30); do
  # --max-time matters: a stale listener on this port (another container sharing
  # --network=host) accepts the connection and never answers, so an untimed curl
  # hangs here forever instead of failing.
  curl -fsS --max-time 2 "http://localhost:$BACKEND_PORT/api/cluster-state" >/dev/null 2>&1 \
    && { ready=true; break; }
  # Backend gone means it died on startup — usually the port is already taken.
  # Report that now rather than burn 30s waiting on a process that exited.
  if ! kill -0 "$backend_pid" 2>/dev/null; then
    echo "backend exited on startup:" >&2
    tail -3 "$ROOT/demo-backend.log" >&2
    echo "if the port is taken, rerun with BACKEND_PORT=<free port>" >&2
    exit 1
  fi
  sleep 1
done
if [ "$ready" = false ]; then
  echo "backend up but not answering on :$BACKEND_PORT after 30s — something else" >&2
  echo "may be holding the port; see $ROOT/demo-backend.log" >&2
  exit 1
fi

if [ "$run_check" = true ]; then
  echo "==> checking demo scenarios still produce their findings"
  python3 "$ROOT/scripts/check-scenarios.py" --base-url "http://localhost:$BACKEND_PORT"
fi

echo "==> freezing /api/* into ui/public/demo"
python3 "$ROOT/scripts/snapshot-api.py" \
  --base-url "http://localhost:$BACKEND_PORT" --out "$SNAPSHOT_DIR"

kill "$backend_pid" 2>/dev/null || true
backend_pid=""

echo "==> building ui (VITE_DEMO_MODE=true)"
cd "$ROOT/ui"
npm install --silent
# VITE_BASE stays unset: locally the site is served from /, not the Pages
# /<repo>/ prefix. Setting it here would 404 every snapshot fetch.
VITE_DEMO_MODE=true npm run build

echo "==> static site ready: $ROOT/ui/dist"
if [ "$serve" = false ]; then
  exit 0
fi

echo "==> serving on http://localhost:$PREVIEW_PORT  (ctrl-c to stop)"
echo "    scenarios: ?s=dns-blackhole ?s=engine-contradiction ?s=mesh-transport ?s=strict-mtls ?s=exposure"
npm run preview -- --port "$PREVIEW_PORT" --host 0.0.0.0

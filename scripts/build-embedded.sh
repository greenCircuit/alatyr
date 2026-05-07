#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> building ui"
cd "$ROOT/ui"
npm install --silent
npm run build

echo "==> building binary"
cd "$ROOT"
go build -tags embed -o graph .

echo "==> done: $ROOT/graph"

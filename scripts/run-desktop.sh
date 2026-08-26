#!/usr/bin/env bash
# GOSO Desktop (SPEC 024). Unsigned darwin build; no notarization / Developer ID.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="${GOPATH_BIN}:${HOME}/go/bin:${PATH}"

if ! command -v wails >/dev/null 2>&1; then
  echo "wails CLI not found. Install: go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0" >&2
  echo "Ensure \$(go env GOPATH)/bin is on PATH." >&2
  exit 1
fi

cd "$ROOT"
MODE="${1:-build}"
case "$MODE" in
  --dev | dev)
    cd desktop
    exec wails dev
    ;;
  --build | build)
    make -C desktop build
    APP="$ROOT/desktop/build/bin/GOSO.app"
    if [[ -d "$APP" ]]; then
      open "$APP"
    else
      echo "built app not found at $APP" >&2
      exit 1
    fi
    ;;
  *)
    echo "usage: $0 [--dev|--build]" >&2
    exit 2
    ;;
esac

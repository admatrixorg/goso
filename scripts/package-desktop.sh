#!/usr/bin/env bash
# GOSO Desktop (SPEC 029). Unsigned zip for darwin/arm64.
# AC-03: do not invoke codesign, notarytool, or altool. No Developer ID, no notarization.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="$ROOT/desktop/build/bin/GOSO.app"
DIST="$ROOT/dist"
ZIP_NAME="GOSO-darwin-arm64-unsigned.zip"
ZIP="$DIST/$ZIP_NAME"
SKIP_WAILS="${SKIP_WAILS:-0}"

STAGE="$(mktemp -d "${TMPDIR:-/tmp}/goso-desktop-zip.XXXXXX")"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

mkdir -p "$DIST"

cat > "$STAGE/README-UNSIGNED.md" <<'EOF'
# GOSO Desktop — unsigned zip (darwin/arm64)

This archive is **unsigned**. There is **no Developer ID**, **no Apple
notarization**, and **no auto-update** (no Sparkle / no in-app updater).

GOSO does not ship a signed DMG. Recipients must accept a Gatekeeper
warning and clear quarantine locally.

## Gatekeeper

macOS will warn that the app is from an unidentified developer. That is
expected for this build.

After unzipping, clear the quarantine flag on the app bundle:

```bash
xattr -cr GOSO.app
```

Then open it once via right-click → Open (or `open GOSO.app`). Do not
expect this zip to pass Gatekeeper as a notarized installer.

## Contents

- `GOSO.app` — Wails unsigned build, when packaged from `desktop/build/bin/GOSO.app`
- `STUB.txt` — written instead of the app when `SKIP_WAILS=1` or the app is missing

This package pipeline never runs `codesign`, `notarytool`, or `altool`.
EOF

INCLUDE_APP=0
if [[ "$SKIP_WAILS" == "1" ]]; then
  echo "==> SKIP_WAILS=1: packaging stub (no GOSO.app, wails not required)"
elif [[ -d "$APP" ]]; then
  INCLUDE_APP=1
else
  echo "==> GOSO.app not found at $APP; packaging stub" >&2
  echo "    Build first with: make -C desktop build" >&2
fi

if [[ "$INCLUDE_APP" == "1" ]]; then
  cp -R "$APP" "$STAGE/GOSO.app"
  echo "==> including GOSO.app from $APP"
else
  cat > "$STAGE/STUB.txt" <<'EOF'
GOSO.app was not included in this zip.

This stub is written when SKIP_WAILS=1 (CI / tests must not require Wails)
or when desktop/build/bin/GOSO.app is missing.

To package a real app:

  make -C desktop build
  ./scripts/package-desktop.sh

The result is still unsigned: no Developer ID, no notarization, no auto-update.
EOF
fi

rm -f "$ZIP"
(
  cd "$STAGE"
  zip -r -X "$ZIP" . -x '*.DS_Store' -x '*/.DS_Store'
)

echo "==> unsigned zip: $ZIP (not notarized, no auto-update)"
unzip -l "$ZIP"

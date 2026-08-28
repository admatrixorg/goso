#!/usr/bin/env bash
# Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.
# Scan docs/qa for banned upstream-author identifiers (CTO-09).
# Tokens are assembled so this script itself stays outside the match list.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

a1="minh"; a2="hai"; a3="phan"
b1="loc";  b2="pham"; b3="nguyen"
AUTHOR_RE="${a1}${a2}${a3}|${b1}${b2}${b3}"

echo "==> agpl-check-docs: docs/qa author identifiers"

if [ ! -d docs/qa ]; then
  echo "==> agpl-check-docs: docs/qa missing" >&2
  exit 1
fi

set +e
hits="$(grep -r -n -i -E -I \
  --exclude-dir=.git \
  -e "$AUTHOR_RE" docs/qa 2>/dev/null)"
rc=$?
set -e
if [ "$rc" -ge 2 ]; then
  echo "==> agpl-check-docs: grep failed (rc=$rc)" >&2
  exit "$rc"
fi
if [ "$rc" -eq 0 ] && [ -n "$hits" ]; then
  echo "==> agpl-check-docs: FAIL — banned author identifier in docs/qa" >&2
  printf '%s\n' "$hits" >&2
  exit 1
fi
echo "==> agpl-check-docs: docs/qa clean (OK)"

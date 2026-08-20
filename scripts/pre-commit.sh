#!/bin/sh
set -e
ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> pre-commit: go vet"
go vet ./... 2>&1

echo "==> pre-commit: gofmt"
if [ -n "$(gofmt -l gateway 2>&1)" ]; then
  echo "gofmt failed:"; gofmt -l gateway; exit 1
fi

echo "==> pre-commit: secret/SAST scan"
# Prefer staged-only gitleaks when available; scan.sh still runs filesystem detect.
if command -v gitleaks >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "==> pre-commit: gitleaks protect --staged"
  cfg="$ROOT/.gitleaks.toml"
  extra=""
  if [ -f "$cfg" ]; then
    extra="--config $cfg"
  fi
  # shellcheck disable=SC2086
  gitleaks protect --staged --redact --verbose $extra --exit-code 1
fi

sh "$ROOT/scripts/scan.sh"
echo "pre-commit: OK"

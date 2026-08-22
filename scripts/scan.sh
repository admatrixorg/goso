#!/bin/sh
# GOSO secret + SAST scan. Fail on findings.
# Local: skip a tool if it is not installed.
# CI (CI=true): both gitleaks and semgrep are required.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CI_MODE=0
if [ "${CI:-}" = "true" ] || [ "${CI:-}" = "1" ]; then
  CI_MODE=1
fi

have() { command -v "$1" >/dev/null 2>&1; }

require_or_skip() {
  tool="$1"
  if have "$tool"; then
    return 0
  fi
  if [ "$CI_MODE" -eq 1 ]; then
    echo "ERROR: $tool is required on CI (install it in the workflow)" >&2
    exit 1
  fi
  echo "==> $tool not installed — skipping (install to enforce scan locally)"
  return 1
}

failed=0

if require_or_skip gitleaks; then
  echo "==> gitleaks"
  cfg="$ROOT/.gitleaks.toml"
  extra=""
  if [ -f "$cfg" ]; then
    extra="--config $cfg"
  fi
  # Scan the working tree (current files), not git history — deterministic for CI + local.
  # shellcheck disable=SC2086
  if ! gitleaks detect --source "$ROOT" --no-git --redact --no-banner --exit-code 1 $extra; then
    echo "gitleaks: FAIL (secret detected)" >&2
    failed=1
  else
    echo "gitleaks: OK"
  fi
fi

if require_or_skip semgrep; then
  echo "==> semgrep"
  rules="$ROOT/.semgrep.yml"
  if [ ! -f "$rules" ]; then
    echo "ERROR: missing $rules" >&2
    exit 1
  fi
  # Local ruleset only — no --config auto (requires login / network, used to be swallowed with || true).
  if ! semgrep --config "$rules" --error --metrics off --quiet; then
    echo "semgrep: FAIL" >&2
    failed=1
  else
    echo "semgrep: OK"
  fi
fi

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "==> scan: OK"

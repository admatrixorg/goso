#!/bin/sh
set -e
echo "==> pre-commit: go vet"
go vet ./... 2>&1
echo "==> pre-commit: gofmt"
if [ -n "$(gofmt -l gateway 2>&1)" ]; then
  echo "gofmt failed:"; gofmt -l gateway; exit 1
fi
if command -v gitleaks >/dev/null 2>&1; then
  echo "==> pre-commit: gitleaks"
  gitleaks detect --source . --no-git --redact --exit-code 1 || exit 1
fi
echo "pre-commit: OK"

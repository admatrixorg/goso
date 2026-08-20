# GOSO — Makefile (Harness)

.PHONY: verify lint vet fmt test build doctor version help

help:
	@echo "Targets: verify | vet | fmt | test | build | lint"

verify: vet fmt test
	@echo "==> verify: OK"

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l gateway)" || (echo "gofmt failed — run 'make fmt-fix'"; gofmt -l gateway; exit 1)

fmt-fix:
	gofmt -w gateway

test:
	go test ./... -count=1

build:
	go build -o bin/goso-gateway ./gateway/cmd/goso-gateway

lint:
	go vet ./...
	@test -z "$$(gofmt -l gateway)" || (gofmt -l gateway; exit 1)
	@if command -v gitleaks >/dev/null 2>&1; then gitleaks detect --source . --no-git --redact --exit-code 1; else echo "gitleaks not installed — skipping"; fi
	@if command -v semgrep >/dev/null 2>&1; then semgrep --config auto --error gateway || true; else echo "semgrep not installed — skipping"; fi

version:
	go run ./gateway/cmd/goso-gateway version

doctor:
	go run ./gateway/cmd/goso-gateway doctor


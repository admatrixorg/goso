# GOSO — Makefile (Harness)

.PHONY: verify lint vet fmt fmt-fix test build doctor version help mcp-verify scan e2e smoke

help:
	@echo "Targets: verify | lint | vet | fmt | test | mcp-verify | scan | e2e | build | smoke"

# vet + fmt + test + mcp + gitleaks/semgrep (if installed) + e2e
verify: vet fmt test mcp-verify scan e2e
	@echo "==> verify: OK"

mcp-verify:
	@echo "==> mcp verify"
	@if [ ! -d mcp/node_modules ]; then pnpm -C mcp install --frozen-lockfile; fi
	pnpm -C mcp verify

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l gateway desktop)" || (echo "gofmt failed — run 'make fmt-fix'"; gofmt -l gateway desktop; exit 1)

fmt-fix:
	gofmt -w gateway desktop

test:
	go test ./... -count=1

scan:
	@sh scripts/scan.sh

e2e:
	@sh scripts/e2e.sh

build:
	go build -o bin/goso-gateway ./gateway/cmd/goso-gateway

lint: vet fmt scan

smoke:
	go run ./gateway/cmd/goso-gateway version
	go run ./gateway/cmd/goso-gateway doctor

version:
	go run ./gateway/cmd/goso-gateway version

doctor:
	go run ./gateway/cmd/goso-gateway doctor

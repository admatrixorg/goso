# GOSO — Makefile (Harness)

.PHONY: verify lint vet fmt fmt-fix test build doctor version help mcp-verify scan e2e smoke package-desktop bench-memory-vi

help:
	@echo "Targets: verify | lint | vet | fmt | test | bench-memory-vi | mcp-verify | scan | e2e | build | smoke | package-desktop"

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

# SPEC 153 bench/memory-vi is part of this test (package gateway/memorybench).
# SQLite always runs. Postgres runs when GOSO_TEST_POSTGRES_DSN is set; CI sets it.
# The bench fails when a case that passed at main 9545d08 fails, or when a case
# that must return nothing admits an item. A score below 100% is the floor, not a failure.
test:
	go test ./... -count=1

bench-memory-vi:
	go test ./gateway/memorybench -count=1 -timeout 15m -v

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

# SPEC 029 unsigned zip (no codesign / notarize / auto-update). SKIP_WAILS=1 writes stub + README-UNSIGNED.md.
package-desktop:
	./scripts/package-desktop.sh

# GOSO Desktop — Wails v2 skeleton (SPEC 009)

.PHONY: verify vet fmt fmt-fix test frontend build help

help:
	@echo "Targets: verify | vet | fmt | test | frontend | build"

# AC-04: green without Wails/CGO (CI-safe). Frontend is built by `make build`.
verify: vet fmt test
	@echo "==> desktop verify: OK"

vet:
	go vet ./internal/...

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt failed — run 'make -C desktop fmt-fix'"; gofmt -l .; exit 1)

fmt-fix:
	gofmt -w .

test:
	go test ./internal/... -count=1

frontend:
	npm --prefix frontend install
	npm --prefix frontend run verify

build: frontend
	@if command -v wails >/dev/null 2>&1; then \
		wails build; \
	else \
		echo "wails CLI not found. Install: go install github.com/wailsapp/wails/v2/cmd/wails@latest"; \
		exit 1; \
	fi

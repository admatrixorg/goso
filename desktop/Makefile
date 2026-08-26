# GOSO Desktop — Wails v2 (SPEC 009 + SPEC 024)

.PHONY: verify vet fmt fmt-fix test frontend build package-desktop help

GOPATH_BIN := $(shell go env GOPATH)/bin
WAILS ?= $(shell command -v wails 2>/dev/null)
ifeq ($(WAILS),)
WAILS := $(GOPATH_BIN)/wails
endif

help:
	@echo "Targets: verify | vet | fmt | test | frontend | build | package-desktop"

# AC-01: green without Wails/CGO (CI-safe). Frontend + darwin app: `make build`.
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
	VITE_DEMO_MODE=false npm --prefix frontend run verify
	@js=$$(ls frontend/dist/assets/*.js 2>/dev/null | head -1); \
	test -n "$$js" || (echo "frontend bundle missing"; exit 1); \
	if grep -q "Trang chủ" "$$js" || grep -q "Vinh Phát" "$$js"; then \
		echo "AC-03 FAIL: DEMO labels leaked into $$js"; exit 1; \
	fi; \
	for label in Agent Phiên Chat "Kết nối" "Nhật ký"; do \
		grep -q "$$label" "$$js" || { echo "AC-03 FAIL: missing tab $$label in $$js"; exit 1; }; \
	done; \
	echo "==> desktop frontend: 5 live tabs, DEMO hidden ($$js)"

# Unsigned darwin app (SPEC 024). No Developer ID, no notarization.
# -m / -nosyncgomod keep go.mod untouched. Local ad-hoc sign may be applied by the toolchain so the .app launches.
build: frontend
	@if [ ! -x "$(WAILS)" ]; then \
		echo "wails CLI not found. Install: go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0"; \
		echo "Ensure \$$(go env GOPATH)/bin is on PATH."; \
		exit 1; \
	fi
	PATH="$(GOPATH_BIN):$$PATH" $(WAILS) build -tags wails -platform darwin/arm64 -m -nosyncgomod
	@echo "==> unsigned app: desktop/build/bin/GOSO.app (not notarized)"

# SPEC 029 unsigned zip (no codesign / notarize). SKIP_WAILS=1 writes stub + README-UNSIGNED.md.
package-desktop:
	../scripts/package-desktop.sh

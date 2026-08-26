# GOSO Desktop (SPEC 009 + SPEC 024)

Wails v2 + React + SQLite local. The window hosts the Control Plane skin and an embedded GOSO gateway (same `gateway/internal/store` + HTTP stack as `goso-gateway`).

## Yêu cầu

- Go 1.25+
- Node 20+ (frontend)
- [Wails v2 CLI](https://wails.io): `go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0` (put `$(go env GOPATH)/bin` on `PATH`)
- macOS: Xcode Command Line Tools (WebKit)

## SQLite + local admin token (SPEC 016 / 024)

Mặc định (ghi đè bằng `GOSO_DB_PATH` / `GOSO_ADMIN_TOKEN_PATH`):

| OS | DB | Token |
|----|----|-------|
| macOS | `~/Library/Application Support/GOSO/goso.db` | `…/GOSO/admin.token` (mode 0600) |
| Windows | `%APPDATA%/GOSO/goso.db` | `…/GOSO/admin.token` |
| Linux | `$XDG_DATA_HOME/GOSO/goso.db` hoặc `~/.local/share/GOSO/goso.db` | same dir |

On first run, desktop generates a random admin token, stores it in that file, and sets `GOSO_ADMIN_TOKEN` for the embedded gateway. **The token is never logged.** Control Plane reads it via the Wails `LocalToken` bind → `localStorage.goso_token`.

- `/healthz` — 200, no token
- `/api/*` — 401 without `Authorization: Bearer <token>` unless `GOSO_DEV_MODE=1`
- Override with `GOSO_ADMIN_TOKEN` in the environment (file is not written in that case)

## Live tabs (VITE_DEMO_MODE=false)

Agent, Phiên, Chat, Kết nối, Nhật ký. DEMO marketing/home tabs are hidden. CRM **Tổng quan** is shown (metrics load when `VITE_GOSOCRM_API_URL` / goso-crm is reachable).

## Dev

```bash
# from repo root
./scripts/run-desktop.sh --dev
# or:
cd desktop && wails dev
```

Cửa sổ desktop mở Control Plane. Request `/api/*` và `/healthz` đi vào gateway nhúng, không cần process `goso-gateway` riêng.

## Build (macOS, unsigned)

**Code-sign, notarization, and a DMG/installer are non-goals.** The `.app` is unsigned (no Developer ID). Gatekeeper may warn; local ad-hoc signing may be applied by the Wails/Xcode toolchain so the binary launches on this Mac. Do not ship this build as a notarized installer.

```bash
./scripts/run-desktop.sh          # wails build + open GOSO.app
make -C desktop build             # npm verify (DEMO gate) + wails build
# binary: desktop/build/bin/GOSO.app
```

`wails build` on darwin/arm64 is expected to exit 0. Put `$(go env GOPATH)/bin` on `PATH` if `wails` is not found.

## Unsigned zip (SPEC 029)

Packaging is a **zip**, not a signed DMG. The archive is **unsigned**: **no Developer ID, no notarization, no auto-update**. Recipients get a Gatekeeper warning; there is no Sparkle / in-app updater.

```bash
./scripts/package-desktop.sh          # from repo root
make package-desktop                  # same
# output: dist/GOSO-darwin-arm64-unsigned.zip
```

The zip always contains `README-UNSIGNED.md` (`xattr -cr GOSO.app`, Gatekeeper, no notarization, no auto-update, no Developer ID). If `desktop/build/bin/GOSO.app` exists it is included; otherwise (or when `SKIP_WAILS=1`) the zip carries `STUB.txt` instead — tests must not require Wails:

```bash
SKIP_WAILS=1 ./scripts/package-desktop.sh
unzip -l dist/GOSO-darwin-arm64-unsigned.zip
```

`scripts/package-desktop.sh` never invokes `codesign`, `notarytool`, or `altool`.

## Verify (không cần Wails/CGO)

```bash
make -C desktop verify    # go vet + gofmt + tests (store reuse + local token auth)
```

Frontend typecheck + Vite build + 5-tab / DEMO-hidden grep: `make -C desktop frontend`.

## Cấu trúc

```
desktop/
├── wails.json
├── main.go                 # Wails window (tag: wails)
├── app.go
├── internal/host/          # DB path, local token, middleware; gọi gateway/internal/*
├── frontend/               # Vite app — Control Plane skin, VITE_DEMO_MODE=false
└── Makefile
scripts/run-desktop.sh      # --dev | default unsigned build + open
scripts/package-desktop.sh  # unsigned zip → dist/GOSO-darwin-arm64-unsigned.zip (SPEC 029)
```

Không duplicate domain: agents/sessions/messages sống trong `gateway/internal/store`. Binary lớn nằm ở `desktop/build/bin/` (gitignored).

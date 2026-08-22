# GOSO Desktop (SPEC 009)

Wails v2 + React + SQLite local. The window hosts Control Plane pages and an embedded GOSO gateway (same `gateway/internal/store` + HTTP stack as `goso-gateway`).

## Yêu cầu

- Go 1.25+
- Node 20+ (frontend)
- [Wails v2 CLI](https://wails.io): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- macOS: Xcode Command Line Tools (WebKit)

## SQLite local

Mặc định (có thể ghi đè bằng `GOSO_DB_PATH`):

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/GOSO/goso.db` |
| Windows | `%APPDATA%/GOSO/goso.db` |
| Linux | `$XDG_DATA_HOME/GOSO/goso.db` hoặc `~/.local/share/GOSO/goso.db` |

## Dev

```bash
# từ repo root
cd desktop
wails dev
```

Cửa sổ desktop mở Control Plane (agents / sessions / chat). Request `/api/*` và `/healthz` đi vào gateway nhúng, không cần process `goso-gateway` riêng.

## Build (macOS)

```bash
make -C desktop build     # npm verify + wails build
# binary: desktop/build/bin/GOSO.app
```

## Verify (không cần Wails/CGO)

```bash
make -C desktop verify    # go vet + gofmt + tests (store reuse)
```

Frontend typecheck + Vite build: `make -C desktop frontend`.

## Cấu trúc

```
desktop/
├── wails.json
├── main.go                 # Wails window (tag: wails)
├── app.go
├── internal/host/          # DB path + middleware; gọi gateway/internal/*
├── frontend/               # Vite app — reuse control-plane/src/pages/*
└── Makefile
```

Không duplicate domain: agents/sessions/messages sống trong `gateway/internal/store`. Binary lớn nằm ở `desktop/build/bin/` (gitignored).

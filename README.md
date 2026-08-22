# GOSO — Gateway AI (clean-room từ ý tưởng GoClaw)

> **Giai đoạn C→B**: (C) Control Plane mới + GoClaw gateway black-box → (B) thay dần bằng GOSO Gateway Go.

## Cấu trúc

```
goso/
├── gateway/          # GOSO Gateway (Go) — thay dần GoClaw gateway ở giai đoạn B
│   ├── cmd/goso-gateway/
│   └── internal/
├── control-plane/    # API quản trị + Dashboard (TypeScript) — giai đoạn C
├── desktop/          # Wails v2 + React + SQLite — giữ lại
├── mcp/              # MCP server (goso-mcp — rebrand goclaw-mcp, 66 tools)
├── docs/             # SETUP, ARCHITECTURE
└── .planning/        # specs, plans, decisions, glossary
```

## Chạy nhanh

```bash
go run ./gateway/cmd/goso-gateway --help
go run ./gateway/cmd/goso-gateway version
go run ./gateway/cmd/goso-gateway doctor
make verify
# MCP (stdio): GOSO_GATEWAY_URL=http://localhost:8080 pnpm -C mcp start
```

Docker (SPEC 012):

```bash
docker compose up --build
# gateway http://localhost:8080  ·  control-plane http://localhost:3000
```

## Tài liệu

- `docs/SETUP.md` — 5 bước dựng môi trường
- `docs/ARCHITECTURE.md` — kiến trúc C→B
- `docs/DEPLOY.md` — Docker Compose + overlay production
- `docs/RUNBOOK.md` — khởi động, backup SQLite, xoay token, sự cố
- `docs/RELEASE.md` — checklist version / changelog / tag
- `CHANGELOG.md` — lịch sử thay đổi
- `.planning/specs/001-harness-goso.md` — SPEC đã LOCKED
- `.planning/specs/009-desktop.md` — Desktop Wails v2 (khung)
- `.planning/specs/013-hardening.md` — SPEC 013 (hardening)
- `desktop/README.md` — `wails dev` / `make -C desktop verify`

```bash
make verify          # vet + fmt + test + mcp + scan + e2e
./scripts/e2e.sh     # healthz → agents → sessions → chat → webhook
./scripts/pre-commit.sh
```

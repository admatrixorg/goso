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
├── mcp/              # MCP server (tận dụng goclaw-mcp, đổi brand sau)
├── docs/             # SETUP, ARCHITECTURE
└── .planning/        # specs, plans, decisions, glossary
```

## Chạy nhanh

```bash
go run ./gateway/cmd/goso-gateway --help
go run ./gateway/cmd/goso-gateway version
go run ./gateway/cmd/goso-gateway doctor
make verify
```

## Tài liệu

- `docs/SETUP.md` — 5 bước dựng môi trường
- `docs/ARCHITECTURE.md` — kiến trúc C→B
- `.planning/specs/001-harness-goso.md` — SPEC đã LOCKED

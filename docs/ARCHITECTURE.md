# ARCHITECTURE — GOSO (C→B)

## Tổng quan

```
Giai đoạn C (6–8 tuần): Control Plane mới + Gateway GoClaw black-box
┌─────────────┐     ┌─────────────────────┐     ┌──────────────┐
│  Control    │────▶│  GoClaw Gateway     │────▶│  Channels    │
│  Plane GOSO │     │  (black-box, WS)    │     │  TG/WS/Zalo  │
│  TS + MCP   │◀────│  Postgres/pgvector  │     │              │
└─────────────┘     └─────────────────────┘     └──────────────┘

Giai đoạn B (3–5 tháng): Thay dần Gateway bằng GOSO Gateway Go
┌─────────────┐     ┌─────────────────────┐     ┌──────────────┐
│  Control    │────▶│  GOSO Gateway (Go)  │────▶│  Channels    │
│  Plane GOSO │     │  clean-room         │     │  TG/WS/Zalo  │
└─────────────┘     └─────────────────────┘     └──────────────┘
                              │
                    ┌─────────┴─────────┐
                    │ Desktop Wails     │
                    │ SQLite FTS5       │
                    │ Knowledge vault   │
                    └───────────────────┘
```

## Nguyên tắc

- **Clean-room**: không copy code GoClaw (CC BY-NC 4.0), chỉ học ý tưởng/hành vi.
- **DDD**: domain (config, health, session...) tách khỏi infra (HTTP, DB, channel).
- **Harness trước**: Makefile/CI/hooks/pre-commit khóa trước khi thêm nghiệp vụ.
- **Security (SPEC 041)**: Bearer `crypto/subtle.ConstantTimeCompare`; `/api` 1MiB; WS 512KiB; injection scan; optional SSRF + workspace jail; AES-256-GCM `secrets` table. SQLite tenancy light — no Postgres `tenant_id` (DI-09).
- **Observability (SPEC 042)**: JSON access log + `/metrics` (SPEC 008). Nested in-memory spans (`agent` / `llm` / `tool`) on `ChatResult` and `GET /api/traces`. OTLP only if `GOSO_OTEL_ENDPOINT` is set (noop otherwise). Grafana SaaS keys = DI-18 (not in git).

## Cấu trúc repo (SPEC 001)

```
goso/
├── gateway/cmd/goso-gateway/  # entry (version/doctor/gateway)
├── gateway/internal/config    # domain config
├── gateway/internal/health    # domain health checks
├── control-plane/             # SPEC 002+
├── desktop/                   # SPEC 009 — Wails v2 + React + SQLite (reuse gateway store)
├── mcp/                       # goso-mcp (rebrand goclaw-mcp, GOSO_GATEWAY_URL)
└── .planning/                 # specs/plans/decisions/glossary
```

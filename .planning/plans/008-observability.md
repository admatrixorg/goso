# PLAN 008 — Observability

> SPEC: `specs/008-observability.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Middleware request_id + structured log | `gateway/internal/observe/log.go` | `go test ./internal/observe -run Log` |
| T02 | LLM trace ring buffer + GET /api/traces | `gateway/internal/observe/trace.go`, handler | `go test ./internal/observe -run Trace` |
| T03 | Stats endpoint | `gateway/internal/observe/stats.go` | `curl /api/stats` |
| T04 | Wire vào gateway | `gateway/cmd/goso-gateway/main.go`, `httpapi` | `go vet ./...` |
| T05 | QA AC 01–05 | `make verify` + e2e | checklist |

## Rationale

- **Ring buffer in-memory**: đơn giản, không DB, đủ observability MVP.
- **Structured JSON log**: dễ grep, không lộ token.
- **Stats endpoint**: không phụ thuộc Prometheus, để sau mới exporter.

## Trạng thái

- [x] T01 — log
- [x] T02 — trace
- [x] T03 — stats
- [x] T04 — wire
- [x] T05 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | `observe.Middleware`: sinh/giữ `X-Request-ID`, log JSON method/path/status/latency, không log query/header (token) |
| AC-02 | ✅ | ring buffer N=200, wrap `llm.Provider`, `GET /api/traces?limit=` newest-first |
| AC-03 | ✅ | `GET /api/stats` JSON (uptime_seconds, request_count, llm_call_count) + `GET /metrics` text |
| AC-04 | ✅ | `go test ./gateway/internal/observe` — middleware, ring, wrap, handlers |
| AC-05 | ✅ | `make verify` OK |

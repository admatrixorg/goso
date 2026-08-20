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

- [ ] T01 — log
- [ ] T02 — trace
- [ ] T03 — stats
- [ ] T04 — wire
- [ ] T05 — QA

# PLAN 010 — Billing & Quota (Token Metering Stub)

> SPEC: `specs/010-billing-quota.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Token estimator | `gateway/internal/billing/estimate.go` | `go test ./internal/billing -run Estimate` |
| T02 | Usage store + GET /api/usage | `gateway/internal/billing/store.go`, handler | `go test ./internal/billing -run Usage` |
| T03 | Wire LLM usage vào trace | `gateway/internal/llm/*`, `channel/*`, `httpapi` | `go vet ./...` |
| T04 | QA AC 01–05 | `make verify` + e2e | checklist |

## Rationale

- **Estimator len/4**: đơn giản, đủ stub metering.
- **Trace + SQLite**: không cần bảng riêng khi mới metering.
- **GET /api/usage**: query theo agent/provider, để sau mới billing VND.

## Trạng thái

- [ ] T01 — estimator
- [ ] T02 — usage api
- [ ] T03 — wire
- [ ] T04 — QA

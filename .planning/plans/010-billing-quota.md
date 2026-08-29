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

- [x] T01 — estimator
- [x] T02 — usage api
- [x] T03 — wire
- [x] T04 — QA

## QA 2026-08-29

Code already present; no recook. Evidence: `docs/qa/010-billing-quota.md`.

| Task | Proof |
|------|--------|
| T01 | `TestEstimateTokens` |
| T02 | `TestUsageQuery`; `GET /api/usage` in `handlers_test.go` |
| T03 | `llm.EstimateUsage` + `recordUsage` on chat |
| T04 | `go test ./gateway/internal/billing ./gateway/internal/llm ./gateway/internal/httpapi -count=1` ok |

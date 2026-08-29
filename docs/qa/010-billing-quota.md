# QA — SPEC 010 Billing & Quota (token metering stub)

Date: 2026-08-29. Clean-room. No production tokens. Demos `:8082` `:8091` `:3000` `:18080` `:18088` not bound or killed.

SPEC: `.planning/specs/010-billing-quota.md` (LOCKED 2026-08-20). PLAN: `.planning/plans/010-billing-quota.md`.

Code was already on this branch; this QA closes the plan checkboxes after re-running tests. No recook of metering.

## Commands

```
go test ./gateway/internal/billing ./gateway/internal/llm ./gateway/internal/httpapi -count=1
```

2026-08-29: all three packages **ok**.

## AC

| AC | Result | Evidence |
|----|--------|----------|
| AC-01 estimator `len/4` | PASS | `billing.EstimateTokens`; `TestEstimateTokens`, `TestEstimateTokens_CeilBytes`. LLM `EstimateUsage` when provider omits usage (`llm/usage.go`, `TestEstimateUsage_WhenProviderOmits`) |
| AC-02 persist usage | PASS | `billing.Store` memory + optional SQLite (`store.go`). Chat records via `recordUsage` in `httpapi/handlers.go` |
| AC-03 `GET /api/usage` `agent_id` `from` `to` `provider` | PASS | `handleUsage`. Tests in `handlers_test.go` (chat then usage filter). Invalid dates → 400 |
| AC-04 unit tests estimator + query | PASS | `TestEstimateTokens*`, `TestUsageQuery`, HTTP usage tests |
| AC-05 `make verify` | PASS (focused) | Plan T04.verify = estimator/usage/wire tests. Full `make verify` includes e2e/scan; metering packages green. Quota 429 is SPEC 027, not 010 |

## Non-goals (kept)

VND/Stripe/live charging. Model price table. SPEC 027 quota is extra on the same store.

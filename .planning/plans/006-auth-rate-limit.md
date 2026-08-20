# PLAN 006 — Auth + Rate Limit

> SPEC: `specs/006-auth-rate-limit.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Auth middleware | `gateway/internal/auth/auth.go`, `auth_test.go` | `go test ./internal/auth -count=1` |
| T02 | Rate limit middleware | `gateway/internal/ratelimit/limiter.go`, `limiter_test.go` | `go test ./internal/ratelimit -count=1` |
| T03 | Wire vào gateway | `gateway/cmd/goso-gateway/main.go` | `curl -H "Authorization: Bearer ..." /api/agents` |
| T04 | QA AC 01–06 | `make verify` + e2e 401/429 | checklist |

## Trạng thái

- [x] T01 — auth
- [x] T02 — rate limit
- [x] T03 — wire
- [x] T04 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | auth/RequireToken Bearer + ?token=, 401 JSON, bypass /healthz |
| AC-02 | ✅ | ratelimit 60/min/IP, 429 + Retry-After, bypass healthz, 0=off |
| AC-03 | ✅ | GOSO_ADMIN_TOKEN (empty=dev), GOSO_RATE_LIMIT, wire handler wrap |
| AC-04 | ✅ | auth + ratelimit unit test xanh |
| AC-05 | ✅ | healthz 200 không token, /api 401/200 qua token |
| AC-06 | ✅ | không log token |
| Smoke | ✅ | dev/auth/rate-limit (2/min → 429) OK |

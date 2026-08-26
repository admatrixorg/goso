# SPEC 027-gw — Gateway quota enforce (GoClaw quota.usage rewrite)

> LOCKED: 2026-08-26. Clean-room. Existing `GET /api/usage` stays. Add quota windows + 429.

## Behavior

Env `GOSO_QUOTA_DAY` (int, 0 or empty = unlimited).  
`GET /api/quota` JSON: `{enabled, requestsToday, inputTokensToday, outputTokensToday, day:{used,limit}}`.  
When enabled and `total_tokens` today ≥ limit, `POST /api/chat` returns **429** `{error:quota_exceeded}` with `Retry-After`. `/healthz` never 429.

## Own

`gateway/internal/billing/*`, `gateway/internal/httpapi/handlers.go` chat path, tests. Do not edit mcp/ or control-plane.

## AC

- [ ] AC-01 usage still 200.
- [ ] AC-02 quota disabled (0) never 429.
- [ ] AC-03 with limit 1, second chat 429.
- [ ] AC-04 `go test ./gateway/...`.

## Non-goals

Stripe, cost USD tables, Grafana.

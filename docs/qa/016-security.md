# QA — SPEC 016 Gateway auth (default-on)

Canonical CRM write-up: sibling **goso-crm** `docs/qa/016-security.md`.

This repo: empty `GOSO_ADMIN_TOKEN` without `GOSO_DEV_MODE=1` → `/api/*` **401**. `/healthz` 200.

Curl 2026-08-26 (port 8098, then stopped):

```
GET /healthz                         → 200 {"ok":true,"version":"0.1.0"}
GET /api/agents                      → 401 {"error":"unauthorized"}
GET /api/agents  Bearer secret-016   → 200 {"agents":[]}
```

`make verify` on this branch. Control-plane `VITE_GOSOCRM_ORG_TOKEN` → header `X-Org-Token`.

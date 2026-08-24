# Demo runbook — control-plane + goso-crm fake

Canonical copy with curl evidence: **goso-crm** `docs/qa/015-demo.md`.

Quick:

```bash
GOSOCRM_FAKE=1 PORT=8089 go run ./cmd/server   # repo goso-crm
cd control-plane && npm run preview -- --port 3000
```

Smoke 2026-08-24: CRM `/healthz` 200, `/readyz` `fake=true`, dashboard title **Tổng quan**; CP preview 200 `ZAgent — GOSO Control Plane`. Servers stopped after curl.

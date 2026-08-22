# Follow-up 2026-08-22 — three sequential jobs (coordinator)

Orca run `run_37db1fcc868b`. Advisor/QC only; workers implemented.

| # | Job | Repo | Branch | Worker commit | Merged `main` |
|---|-----|------|--------|---------------|----------------|
| 1 | SPEC 014 Connector Architecture | `admatrixorg/goso` | `admatrixmdp/spec014` | `bff48b2` | `12eb09c` |
| 2 | Dockerize goso-crm | `admatrixorg/goso-crm` | `admatrixmdp/spec015-deploy` | see that repo `docs/qa/015-deploy.md` | `e7aa4dc` |
| 3 | Control Plane CRM metrics | `admatrixorg/goso` | `admatrixmdp/control-plane-crm-metrics` | `54d9cfb` | `8b7356c` |

## V1 — SPEC 014 (goso core)

- Base `main` @ `e11755c`. Clean-room connector registry + MCP/HTTP + approval + EventStore + Control Plane Connectors/Events.
- **No** ZaloCRM/AGPL code in goso; remote HTTP/MCP to goso-crm only (`GOSOCRM_API_URL` default `http://127.0.0.1:8089`).
- Worker QC: `make verify`, `npm run typecheck` + `build`, `./scripts/e2e-connector.sh`.
- Report: `docs/qa/014-qa.md`.

## V3 — Control Plane metrics (this repo)

- Tab **CRM metrics** fetches `GET /api/crm/metrics` + `GET /api/crm/advisor` with `X-Org-ID`.
- KPI: messagesSent / messagesReceived / unreplied / avgResponseTime / kpiCompletionRate / revenueMonth (+ sampleDays).
- Health: `/healthz` or `/readyz`, 3s timeout → **goso-crm online/offline**.
- Coordinator re-verify: typecheck + build + `make verify` green @ `54d9cfb`.
- Report: `docs/qa/014-control-plane-crm-metrics.md`.

SPEC 008–013 remain on their branches; not merged.

# QA — Control Plane CRM metrics (SPEC 007 + 014/015)

Date: 2026-08-22
Branch: `admatrixmdp/control-plane-crm-metrics`
Base: `main` @ `12eb09c` (SPEC 014 merge). Did **not** restack 008–013.

Control Plane shows live KPI from **goso-crm HTTP**. No goso-crm Go import. No AGPL copy.

## Commands

```
cd control-plane && npm run typecheck && npm run build   # OK (vite v5.4.21, 38 modules)
make verify                                               # OK (vet + fmt + go test ./...)
```

## AC

| Item | Result | Evidence |
|------|--------|----------|
| Tab **CRM metrics** keeps Agents, Sessions, Chat, Connectors, Events | PASS | `control-plane/src/App.tsx` |
| `GET {base}/api/crm/metrics` + `GET {base}/api/crm/advisor` with `X-Org-ID` | PASS | `control-plane/src/api/crm.ts` |
| KPI: messagesSent, messagesReceived, unreplied, avgResponseTime, kpiCompletionRate, revenueMonth, sampleDays | PASS | `control-plane/src/pages/CrmMetrics.tsx` |
| Online/offline via `/healthz` or `/readyz`, timeout ~3s, no hang | PASS | `crmHealth()` AbortController 3000ms |
| Empty/zero metrics is valid | PASS | zeros mapped with `num()`; not treated as error |
| `VITE_GOSOCRM_API_URL` default `http://127.0.0.1:8089` | PASS | `CRM_UPSTREAM_DEFAULT`; dev uses `/crm-api` proxy to that origin |
| `VITE_GOSOCRM_ORG_ID` default test-a `01a01fe5-704c-7375-aa1f-6e50a9d0296d` | PASS | `CRM_ORG_DEFAULT` |
| Vite proxy `/crm-api` → goso-crm (CORS-free dev) | PASS | `vite.config.ts` rewrite; documented in `control-plane/README.md` |
| No secrets in UI/source/network display | PASS | CRM client does not send gateway Bearer; errors redact `Bearer …` |
| No AGPL / no goso-crm Go import | PASS | `go.mod` unchanged; grep `goso-crm` module path empty |
| `npm run typecheck` + `npm run build` + `make verify` | PASS | this report |

## Notes

- CRM HTTP client is separate from the gateway client so the admin token is never attached to goso-crm fetches.
- In `npm run dev`, unset `VITE_GOSOCRM_API_URL` uses same-origin `/crm-api` (proxy → `http://127.0.0.1:8089`). Production/preview without a reverse proxy should set the full URL at build time.
- Live goso-crm was not required for typecheck/build/verify. Offline UI is the expected state when CRM is down.

## Coordinator QC (2026-08-22)

Re-ran on worktree `/Users/mqglobal/orca/workspaces/goso/cp-crm-metrics` @ `54d9cfb`:

| Check | Result |
|-------|--------|
| `cd control-plane && npm run typecheck` | PASS (`tsc --noEmit`) |
| `cd control-plane && npm run build` | PASS (vite 5.4.21, 38 modules, `dist/assets/index-D4Arpm2x.js`) |
| `make verify` | PASS (`go vet` + `gofmt` + `go test ./... -count=1`) |
| Tabs Agents / Sessions / Chat / Connectors / Events kept | PASS (`App.tsx`) |
| KPI fields + advisor + `X-Org-ID` | PASS (`crm.ts`, `CrmMetrics.tsx`) |
| Online/offline `/healthz` then `/readyz`, 3s abort | PASS |
| No goso-crm Go import / no AGPL authors | PASS (`go.mod` unchanged; grep empty) |
| No secrets in CRM client | PASS (no gateway Bearer; `Bearer …` redacted in errors) |
| SPEC 008–013 branches not restacked | PASS (base `12eb09c` only) |

Config is `VITE_GOSOCRM_API_URL` / `VITE_GOSOCRM_ORG_ID` (Vite client bundle). Default upstream `http://127.0.0.1:8089`, org test-a. Dev uses `/crm-api` proxy.

**Merge:** `8b7356c` on `main` (pushed `origin/main`). Feature tip `f2ac162` (worker `54d9cfb` + coordinator QC).

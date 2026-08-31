# QA — SPEC 128 Per-menu URLs + CRM proxy 8082

Date: 2026-08-31. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` were not bound or killed. No vendor/S3/Grafana/SSO/channel success. No token literals, CRM passwords, or token-file contents in this record.

## Live surfaces inspected (worker, before repo edits)

| Surface | URL | Observation |
| --- | --- | --- |
| Control Plane | `http://127.0.0.1:3000/` | 200. Vite demo. Active menu is React `tab` state; only `#traces` is special-cased. |
| Health (proxy) | `http://127.0.0.1:3000/healthz` | 200 `{"ok":true,...}`. |
| CRM health via demo proxy | `http://127.0.0.1:3000/crm-api/healthz` | 200 `{"status":"ok"}` — running demo Vite already targets `:8082`. |
| CRM metrics, no org token | `http://127.0.0.1:3000/crm-api/api/crm/metrics` | 401 `{"error":"unauthorized"}` (DI; not a dead-port 500). |
| CRM metrics + X-Org-ID only | same + `X-Org-ID` | 401 (org token still required; not invented). |
| Direct CRM | `http://127.0.0.1:8082/healthz` | 200 `{"status":"ok"}`. |
| Gateway health | `http://127.0.0.1:18080/healthz` | 200 `{"ok":true,...}`. |
| Agents without bearer | `http://127.0.0.1:3000/api/agents` | 401 `{"error":"unauthorized"}`. |

Repo `control-plane/vite.config.ts` and `crm.ts` still defaulted to `:8089` (nothing listens). `/tmp/goso-cp-demo/vite.config.ts` already proxies `/crm-api` → `:8082`. Worker must still fix the **repo** so the next `:3000` restart is correct.

Left-nav GET probes without a bearer (status only; bodies not copied except the public error shape already shown): every cited list path (`/api/agents` … `/api/tts`, `/api/tools`, `/crm-api/api/settings/users`) returned **401**, not 404/500. `/api/mcp` also 401 on the gateway; the MCP menu still uses ConnectorPanel `/api/connectors` and does not call `/api/mcp`. KG graph is not probed without `agent_id`.

HTTP probes did not send a gateway bearer and did not read any token file. CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, Vite `:3000`, gateway `:18080` left running. Nothing listens on `:8089`.

## What changed

Tiny hash parse/serialize in `control-plane/src/api/hash-route.ts`. `App.go()` writes the hash (`location.hash`) and is the command-palette helper. Load / `hashchange` / `popstate` restore the tab. Unknown hashes `replaceState` to `#/overview`. Old `#traces` / `#traces/<id>` rewrite to `#/traces` / `#/traces/<id>`. Config subpages write `#/config/<page>` (`#/config` = account). Demo hashes parse only when `VITE_DEMO_MODE`.

CRM `CRM_UPSTREAM_DEFAULT` and Vite `/crm-api` target `http://127.0.0.1:8082`. Rewrite `/crm-api` → upstream path kept. No org token added. Heatmap no longer renders `heat.empty` while a CRM error is showing. Advisor chrome still uses `crmAdvisorChrome` so blocking CRM is not “0 tips”.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 295/295 pass (includes `hash-route.test.ts`: every live tab round-trip, `#/`/`#/overview`, unknown rewrite, old traces aliases, settings pages, demo-only tabs ignored in live, functions→tools; CRM default is `:8082`; traces hash helpers emit `#/traces`).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## Out of scope

Merge `--no-ff` and Vite `:3000` restart belong to advisor/CTO live QC. CRM `:8082` and sidecar `:8091` untouched. Hash-router click/refresh/back-forward in the browser waits for the post-merge `:3000` restart. After restart, CRM health should stay 200 through `/crm-api`; metrics without an org token must stay 401 with unauthorized chrome, never fake zeros or vendor success.

No credentials or secret values are included in this record.

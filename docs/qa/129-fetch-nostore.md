# QA — SPEC 129 Gateway fetches must not reuse HTML cache

Date: 2026-08-31. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` were not bound or killed. No vendor/S3/Grafana/SSO/channel success. No token literals or token-file contents in this record.

## Live surfaces inspected (worker, before repo edits)

Verified by the dispatch (after SPEC 127 token save; chrome Gateway · connected; `localStorage.goso_token` set; length not copied here):

| Surface | Observation |
| --- | --- |
| Curl `GET http://127.0.0.1:3000/api/agents` with Bearer | 200 `application/json`. |
| Same-tab `fetch("/api/agents")` without `cache: "no-store"` | 200 `text/html` (`<!doctype html>`, Vite index). |
| Same-tab `fetch(..., { cache: "no-store" })` | 200 `application/json`. |
| Overview / Agents | `agents: non-JSON · sessions: non-JSON · channels: non-JSON`; kind degraded; Agents Create disabled. |

Cause: `jsonFetch` omitted `cache`, so the browser reused the Vite HTML document. Not a gateway 401 and not a missing token.

HTTP probes in this worker run did not print token values or token-file contents. CRM `:8082` and sidecar `:8091` left running. Vite `:3000` not restarted.

## What changed

Shared `gatewayFetchInit` / `readGatewayJson` in `control-plane/src/api/gateway-http.ts`. Default `cache: "no-store"` unless the caller already passed `cache`. After `res.ok`, HTML `Content-Type` or a `<!doctype` body throws `non-JSON response` and is never parsed as `{agents:[]}`. Wired through `jsonFetch`, `probeHealthz`, `probeStats`, `chatStream`, storage blob/upload, backup download, and events/logs streams. CRM client unchanged. No new i18n. No token logging.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 303/303 pass (includes `gateway-http.test.ts`: default `cache: "no-store"`, caller cache preserved, mock fetch captures `init.cache === "no-store"`, HTML 200 does not parse as `{agents:[]}`, doctype body rejected even with JSON content-type, legitimate empty `{agents:[]}` still parses; `formatPublicError` maps HTML/`non-JSON response` Errors to the same public string; Overview HTML list failure is degraded not empty inventory).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## Out of scope

Merge `--no-ff` and Vite `:3000` restart belong to advisor/CTO live QC. CRM `:8082` and sidecar `:8091` untouched. CRM org token 401, channel vendor tokens, S3/Grafana/SSO stay DI. Live browser confirmation that Overview lists load as JSON waits for the post-merge `:3000` restart.

No credentials or secret values are included in this record.

## Advisor live QC

Date: 2026-08-31. After `worker_done` on `task_1587516fd7e4` / `ctx_09badafe3d3b`. Merged `--no-ff` as `a41237d` (`Merge SPEC 129 fetch cache no-store`) of `58df4e8` + `f2daeeb` onto `28eddd7`. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No token literals in this record.

Restart: Vite `:3000` only (new listen pid `5940`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`.

Advisor re-ran `npm test` (303/303), `npm run typecheck`, `npm run build` on the worker worktree before merge. `agpl-check` and `agpl-check-docs` exit 0.

Browser (Orca tab, `goso_token` present — length/value not recorded):

| Check | Live | Verdict |
| --- | --- | --- |
| Overview | `#/overview`. Chrome `Gateway · connected`. Body does **not** contain `non-JSON`. | PASS |
| Agents | `#/agents`. No `non-JSON`, no unauthorized, no doctype dump. | PASS |
| `fetch("/api/agents", { cache: "no-store" })` + bearer | 200 `application/json`, body looks like JSON, not HTML. | PASS |
| Bare `fetch("/api/agents")` (no cache option) | Still 200 `text/html` Vite index — browser document cache. App paths now force `no-store`, so UI is not affected. | PASS (expected leftover for uncached-option callers) |

### Advisor verdict: PASS — SPEC 129 closed

Do not spawn a second 129 worker. CRM `:8082` and sidecar `:8091` remain untouched.

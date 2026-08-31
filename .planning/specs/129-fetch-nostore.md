# SPEC 129 — Gateway fetches must not reuse HTML cache

Status: implemented (worker). Merge `--no-ff` and Vite `:3000` restart stay with advisor/CTO after SPEC 128 advisor live QC PASS (`28eddd7`).
Owner: Grok implementation; advisor/CTO merge and live QC.
Base: `origin/main` at `28eddd7`.

## Goal

Control Plane gateway GETs must not reuse the Vite document cache. A cached `index.html` 200 must never parse as empty inventory (`{agents:[]}`) or look like a successful list.

## Operator question

“Gateway chrome is connected and the admin token is saved, but Overview still says `agents: non-JSON` / `sessions: non-JSON` / `channels: non-JSON` and Agents Create is disabled.”

## Cause (verified)

After SPEC 127 token save, `localStorage.goso_token` is set and chrome shows Gateway · connected. Curl `GET /api/agents` with Bearer returns **200 application/json**. Same-tab `fetch("/api/agents")` without `cache: "no-store"` returns **200 text/html** (Vite `index.html`). `fetch(..., { cache: "no-store" })` returns **200 application/json**.

`jsonFetch` in `control-plane/src/api/client.ts` did not set `cache`. `res.json()` then threw; `formatPublicError` mapped doctype to **non-JSON response**. Overview treated the throw as list errors (degraded), not 401. This is a Control Plane fetch-cache bug, not a gateway 401 and not a missing token.

## Behavior

1. `jsonFetch` and every other Control Plane helper that talks to the gateway (`probeHealthz`, `probeStats`, `chatStream`, storage `blobFetch` / upload, backup download, events/logs GET streams) default `cache: "no-store"` unless the caller already passed `cache`.
2. After `res.ok`, if the body is expected JSON: `Content-Type` HTML / `text/html` or a body starting with `<!doctype` throws the existing public error `non-JSON response`. It is never coerced to `{agents:[]}` / empty success.
3. CRM `crm.ts` is unchanged (CRM org token 401 stays DI).
4. No new vendor-success i18n. Do not log tokens.

## Constraints

- Clean-room React/TS. Do not copy GoClaw/Dewee source.
- Secrets never in chat/QA/git (no token literals, no `admin.token` contents).
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Non-goals

- CRM org token 401, channel vendor tokens, S3/Grafana/SSO.
- Auto-fill secrets from a local token file.
- Copying GoClaw layouts/source.
- Merge `--no-ff` and Vite restart (advisor/CTO).

## Acceptance criteria

1. Gateway helpers default `cache: "no-store"` unless the caller set `cache`.
2. HTML 200 (content-type or `<!doctype`) throws `non-JSON response` and does not parse as `{agents:[]}`.
3. Unit tests mock fetch and capture `init.cache === "no-store"`; HTML 200 is rejected.
4. No new vendor-success copy. Tokens are not logged.
5. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
6. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0.
7. Delivery is two commits (feat vs docs) like SPEC 127/128. Merge stays with advisor/CTO.

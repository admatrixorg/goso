# QA — SPEC 111 Logs

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Logs: live tail with component selector, text filter, DEBUG/INFO/WARN/ERROR chips; start/stop, clear local view | `docs/qa/090-goclaw-sidebar-ux.md` Logs |

goso mapping (self-written): live tab `logs` in [App.tsx](../../control-plane/src/App.tsx) renders [LogsPage](../../control-plane/src/pages/LogsPage.tsx). Listing is `GET /api/logs` (`/v1/logs` alias) with `component`/`q`/`level` plus `limit`/`after` (default 50, max 200, UI seed 100). Live stream is `GET /api/logs/stream` (SSE `event: log`, `Last-Event-ID` / `after` replay, 15s ping). The live socket is unfiltered; component/text/severity filters apply in the browser so a filter change does not skip seqs. Pause/resume disconnects or resumes the stream; clear-local-view drops only the browser list and keeps the seq cursor so resume does not refill. Slow subscribers are disconnected so they reconnect and replay. Ring default 1024 in serve (min 32). GET/stream messages drop secret JSON keys (`token`, `authorization`, …) and token/`token=` shapes (`sk-`, `Bearer`, `xai-`, `AIza`, `gsk_`, `wh_`). View-token GET list/stream 200; POST is not a route (403 via view matrix). Access logs from the observer middleware feed the ring (path only, never query/headers).

Out of scope: Tenants (112). API Keys (113). Copying GoClaw chrome. Live vendor tokens. SPECs 112–118.

## What changed

- Live nav tab + page. Component/text/severity filters. Pause/resume/clear-local-view. Loading / empty / error / reconnect-backoff (1s…15s). Initial empty state instructs Start.
- Bounded server ring plus client live cap 200. Server-side redaction. GET/stream never returns credentials.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/111-logs.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/logstore ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/observe ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicLog` dropping `token`/`Authorization`/Bearer/`sk-`/`token=` shapes, `publicHasSecrets`, `mergeLive` cap 200 + seq dedupe, filters, `backoffDelay` 1s…15s, `parseSseBlock`.
- `go test` logstore: ring cap, secret keys dropped, token/`token=` shapes redacted, component/text/level filters, `after` cursor, slow-sub drop. httpapi: GET omits `sk-`/`token`/`Bearer`; SSE live log + `Last-Event-ID` replay; `/v1/logs` alias; view-token GET 200 / POST 403. auth/serve: view GET `/api/logs` and `/api/logs/stream` 200, POST 403. observe: access-log tail uses path only (no query/token).
- Page copy: Start/Stop, pause/resume, clear view; “Start to tail gateway logs.” / “Bấm Bắt đầu để theo dõi log gateway.” Stream is plain text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Tenants (112). API Keys (113). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 112-118.

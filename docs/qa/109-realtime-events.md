# QA — SPEC 109 Realtime Events

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Realtime Events: live stream with team/agent and event-type filters; pause/resume, clear local view, waiting state | `docs/qa/090-goclaw-sidebar-ux.md` Realtime Events |

goso mapping (self-written): live tab `events` in [App.tsx](../../control-plane/src/App.tsx) still renders [EventsPage](../../control-plane/src/pages/Events.tsx). Historical list remains `GET /api/events` (`/v1/events` alias) with `kind`/`connector` plus `type`/`actor` filters and a 500-cap ring (default list 50, UI 100). Optional live stream is `GET /api/events/stream` (SSE `event: ops`, `Last-Event-ID` / `after` replay, 15s ping). The live socket is unfiltered; type/actor/kind/connector filters apply in the browser so a filter change does not skip seqs. Pause/resume disconnects or resumes the stream; clear-local-view drops only the browser live list and keeps the seq cursor so resume does not refill. Slow subscribers are disconnected so they reconnect and replay. Event types are `connector`, `agent`, `team`, `task`, `message`, `agent_link`. Actor matches `actor`, `agent_id`, or `team_id`. Row expand is schema-safe (allowlisted keys only, no HTML). GET/stream summaries drop message/tool payload keys (`body`, `arguments`, `prompt`, …) and token shapes (`sk-`, `Bearer`, `xai-`, `AIza`, `gsk_`, `wh_`). View-token GET list/stream 200; POST is not a route (403 via view matrix).

Out of scope: Activity (110). Logs (111). Tenants (112). Copying GoClaw chrome. Live vendor tokens. SPECs 110–118.

## What changed

- Optional live SSE on EventsPage plus bounded historical connector list. Pause/resume/clear-local-view. Type and actor filters. Loading / empty / error / reconnect-backoff (1s…15s).
- Server ring (default 1024 in serve, min 32) plus client live cap 200. Schema-safe detail. GET/stream never returns message/tool payload secrets. Agent/team/task/message/link mutations emit operator events; team message bodies are not copied into events.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/109-realtime-events.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/eventstore ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/agent ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicEvent` dropping `body`/`arguments`/token values, `publicHasSecrets`, `parseDetail` skipping payload and nested objects, `mergeLive` cap 200 + seq dedupe, `backoffDelay` 1s…15s, `parseSseBlock`.
- `go test` eventstore: ring cap, subscribe, type/actor/after query, payload keys dropped, token shapes redacted. httpapi: GET omits team-message body and `sk-`/`Bearer` values; agent/team/task/message/link types on GET; SSE live ops + `Last-Event-ID` replay; `/v1/events` alias; view-token GET 200 / POST 403. auth/serve: view GET `/api/events` and `/api/events/stream` 200.
- Page copy: live on/off, pause/resume, clear view; “Waiting for live events.” / “Đang chờ sự kiện live.” Expand is `<dl>` text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Activity (110). Logs (111). Tenants (112). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 110-118.

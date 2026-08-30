# SPEC 125 — MONITORING operator UI/UX

Status: queued after SPEC 124 CTO/advisor QC
Owner: Grok implementation; Codex CTO architecture, merge, and live QC (advisor live-QC while Codex credits exhausted)
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's MONITORING pages answer the same operator questions and cover the same operational states as live Dewee without copying pixels, wording, CSS, components, or source. Scope is Traces, Realtime Events, Activity, and Logs. Reuse the accepted PageChrome / PageStatus / `classifyPageState` / `inventoryBlocksMutation` pattern from SPEC 120–124. Preserve goso's stronger redaction, public-shape, pause/resume, and bounded-retention contracts. Do not invent Grafana/OTEL Cloud success.

## Live surfaces and APIs

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Traces | `http://127.0.0.1:18791/traces` | App tab `traces`, `control-plane/src/pages/TracesPage.tsx` | list/detail `/api/traces`, `/api/traces/:id` via `api/traces.ts`, `api/traces-ops.ts` |
| Realtime Events | `http://127.0.0.1:18791/events` (also `/realtime-events`) | App tab `events`, `Events.tsx` | historical list `/api/events`; SSE `/api/events/stream` via `api/events.ts`, `api/events-ops.ts` |
| Activity | `http://127.0.0.1:18791/activity` | App tab `activity`, `ActivityPage.tsx` | paginated audit list `/api/activity` via `api/activity.ts`, `api/activity-ops.ts` |
| Logs | `http://127.0.0.1:18791/logs` | App tab `logs`, `LogsPage.tsx` | historical list `/api/logs`; SSE `/api/logs/stream` via `api/logs.ts`, `api/logs-ops.ts` |

Do not connect Grafana Cloud, OTEL exporters, paid log vendors, or claim live stream success when SSE is unauthorized/unsupported. Pause/resume/clear-local-view must not delete server history. Never add a live-looking action for a Dewee behavior that goso's cited APIs do not support. Do not add export unless an existing goso route backs it.

## Constraints

- Reuse PageChrome / PageStatus / classifyPageState / inventoryBlocksMutation. Do not fork a second incompatible pattern.
- Error or permission must never render simultaneous zero-count empty claims or enabled privileged actions.
- Preserve last-known data only when clearly labeled stale with last successful refresh time.
- Historical list failure and SSE failure may be shown separately only with clear provenance.
- Credential/secret invariant: never render, log, screenshot, fixture, or write to QA a token, prompt, tool argument blob, HMAC, password, or raw vendor/OTEL error that could contain one.
- Complete i18n in Vietnamese and English.
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Non-goals

- Copying Dewee pixels, wording, CSS, components, or source.
- Inventing Grafana/OTEL Cloud, paid log vendor, or live-stream success without configured evidence.
- Mixing Activity audit records into Events.
- Export of traces/events/activity/logs unless a cited API exists.
- Merge and Vite restart (Codex CTO / advisor live QC).

## Acceptance criteria

1. Traces, Events, Activity, and Logs open with consistent page chrome, primary action, refresh, and filters where relevant.
2. Loading, true empty, filtered empty, generic error, permission, stream/history partial failure, and stale are independently testable and never contradict one another.
3. Traces answers list/detail/span/token/latency questions using existing APIs with redaction and truncated/error-group honesty.
4. Events distinguishes historical vs live vs paused vs reconnecting; clear-local-view is local; Activity remains a separate audit surface.
5. Activity pagination/filters are permission-gated and public-shaped; no fake export/live stream.
6. Logs distinguishes history vs SSE; pause/resume/clear-local are honest; no credential lines.
7. No Grafana/OTEL Cloud, paid log vendor, or live-stream success is claimed without factual configured evidence.
8. Vietnamese and English are complete.
9. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
10. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
11. Delivery is merged to `main` with `--no-ff` after SPEC 124 passes live QC, then only Vite `:3000` is restarted.

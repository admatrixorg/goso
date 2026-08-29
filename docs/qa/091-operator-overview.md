# QA — SPEC 091 Operator gateway Overview

Date: 2026-08-29. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. Existing `:3000` belongs to the demo checkout and was left running. Do not paste goclaw Go. Do not merge.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Overview is gateway operations (health, traffic, agents/channels, sessions), not a CRM-only dashboard | `docs/qa/090-goclaw-sidebar-ux.md` Overview row |

goso mapping (self-written): live tab `crm` in [App.tsx](../../control-plane/src/App.tsx) now renders [OverviewPage](../../control-plane/src/pages/OverviewPage.tsx). The page composes existing `GET /healthz`, `GET /api/stats` (`uptime_seconds`, `request_count`, `llm_call_count`, `ws_up`, `last_heartbeat`), `GET /api/agents`, `GET /api/sessions`, `GET /api/channels` (health buckets only). Optional `GET /api/cron` count when the list succeeds. Poll interval 15s. CRM stays a drill-down card via existing `CrmMetricsPage`. Heatmap stays its own tab.

Out of scope: full usage charts, cost accounting, request tables with bodies, new Go overview endpoint, live vendor tokens.

## What changed

- Control-plane Overview: KPIs for gateway state, uptime, requests, LLM calls, `ws_up`; cards for agent count, session count, channel health (running/missing/failed/parked), last heartbeat.
- `CrmMetricsPage` kept; title is CRM metrics; rendered as `embedded` drill-down on Overview. Not deleted.
- i18n vi+en (`overview.*`, `crm.title` retitled). Loading / empty / error / degraded / unauthorized. Channel GET is counted by `health`/`missing` only — env names and secret-shaped extra keys are dropped.
- `probeStats` now parses the existing stats JSON fields (chrome still uses `lastHeartbeat`).
- No Go public API change. Existing `observe.Stats` fields cover the KPIs.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/observe ./gateway/internal/channel ./gateway/internal/health -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge.

## Proof

- `npm run typecheck` exit 0. `npm test` 10/10 (healthKind + parseStatsBody + channel health buckets + overview kind + uptime).
- `go test` observe/channel/health exit 0. `GET /api/stats` JSON tags `uptime_seconds`, `request_count`, `llm_call_count`, `ws_up`, `last_heartbeat` unchanged (`TestHandleStatsAndMetrics`, `TestStats_WsUp`).
- Channel catalog GET still returns env **names**, never token values (`Catalog` / existing channel secret tests). Overview counts `health` only.
- `agpl-check` and `agpl-check-docs` exit 0.
- Live tab remains `tab=crm` → OverviewPage; heatmap still `tab=heatmap`. CRM card `data-overview-crm`.

## Non-goals

Usage charts / cost. Request body table. New `/api/overview`. Heatmap merge. Binding/killing demo ports. Merge. Copying goclaw Go. Inventing live vendor tokens.

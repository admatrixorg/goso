# SPEC 091 — Operator gateway Overview

> After 090. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue source: `.planning/specs/090-sidebar-ux-queue.md` row 091. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Overview.

## Goal

Live tab **Overview** (`tab=crm` today) becomes an **operator gateway overview**. CRM org metrics stay as a **drill-down card**, not the whole page.

## Compose (existing APIs first)

Use GET `/healthz`, GET `/api/stats` (`uptime_seconds`, `request_count`, `llm_call_count`, `ws_up`, `last_heartbeat`), GET `/api/agents`, GET `/api/sessions`, GET `/api/channels` (health badges, no secrets). Optional cron/list if already exposed. No request payload dumps. Bounded poll (~15s). Loading / empty / error / degraded / unauthorized.

## AC

- [ ] Overview heading + KPIs: gateway connected/degraded/offline, uptime, requests, LLM calls, `ws_up`.
- [ ] Cards: agents count, sessions count, channels health (running/missing/failed/parked), last heartbeat.
- [ ] CRM metrics remain reachable (existing `CrmMetrics` block or link), not deleted.
- [ ] GET never shows tokens. i18n vi+en. CP typecheck. Tests for any new/changed API fields. agpl 0.

## Out of scope

Full GoClaw Usage charts, cost accounting, request table with bodies. Heatmap stays its own tab.

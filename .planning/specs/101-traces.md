# SPEC 101 — Traces operator surface

> After 100. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 101. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Traces.

## Goal

Complete **TracesPage**: searchable/filterable traces, trace detail and spans, token/latency/status summaries, time ranges, pagination, useful error grouping. Redact prompt/tool/result secrets. Tenant isolation. Bounded payload loading. Stable trace links. Loading/empty/partial-data states.

## AC

- [ ] List: search/filter (agent/channel/status if fields exist), time range, pagination. Loading/empty/error.
- [ ] Detail: spans, token/latency/status. Redact prompt/tool/result secrets. Bounded payload.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/101-traces.md`.

## Out of scope

Logs page (111). Activity audit (110). Copying GoClaw dialogs. Live vendor tokens.

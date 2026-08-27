# SPEC 046 — Control-plane live-tab states (loading / empty / error)

> LOCKED: 2026-08-27. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

## Goal

Live sidebar tabs (Agents, Sessions, Chat, Teams, Vault, Memory, Providers, Channels, Webhooks, Traces, Functions, Connectors, Events) must show **loading**, **empty**, and **error** instead of a blank pane. Chat must surface gateway 502 text (e.g. LLM 401) in the error row — do not swallow.

DEMO-only tabs (`home` `tasks` `meetings` `friends` `calendar` `gallery`) stay behind `VITE_DEMO_MODE`. Do **not** wire mock data as live CRM. Do not remove them.

## Do

- Shared small `StatusLine` or reuse existing `EmptyState` + a loading label (`t("common.loading")` / `common.error`) in **both** vi.ts and en.ts.
- Each live page: `loading` boolean on first fetch; error string already exists on most pages — show it near the header; empty already exists — keep.
- `ChatPage`: on send failure, keep the user bubble and show assistant-side error from `String(e)` (truncate, no secrets).
- `npm run typecheck` must pass.

QA: `docs/qa/046-cp-live-states.md`. Commit `admatrixmdp/spec046-cp-live-states`. Do not merge.

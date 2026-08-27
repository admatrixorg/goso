# QA — SPEC 046 Control-plane live-tab states

Date: 2026-08-27. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

## What changed

Live sidebar tabs no longer render a blank pane on first fetch. Shared `StatusLine` (`control-plane/src/ui/StatusLine.tsx`) shows `t("common.loading")` / `t("common.error")` plus the existing error string. Per-page `EmptyState` copy is unchanged. DEMO tabs (`home` `tasks` `meetings` `friends` `calendar` `gallery`) stay behind `VITE_DEMO_MODE` in `App.tsx` — mocks are not wired as live CRM.

| Tab | Loading | Empty | Error |
|-----|---------|-------|-------|
| Agents | first `GET /api/agents` | `agents.empty` | header `StatusLine` |
| Sessions | first `GET /api/sessions` (full + compact) | `sessions.empty` | header `StatusLine` |
| Chat | first `GET /api/sessions/{id}/messages` | `chat.empty` / `chat.emptySession` | header `StatusLine` **and** assistant bubble on send fail |
| Teams | first `GET /api/teams` | `teams.empty` | header `StatusLine` |
| Vault | first `GET /api/vault/docs` | `vault.empty` | header `StatusLine` |
| Memory | first sessions fetch; notes fetch when session picked | `memory.empty` | header `StatusLine` |
| Providers | first `GET /api/providers` | `providers.empty` | header `StatusLine` |
| Channels | first `GET /api/channels` | `channels.empty` | header `StatusLine` |
| Webhooks | while `POST /api/webhooks` (no GET list) | `webhooks.empty` | header `StatusLine` (secrets redacted) |
| Traces | first `GET /api/traces` | `traces.empty` / `traces.emptySpans` | header `StatusLine` |
| Functions | first agents/connectors fetch; tools fetch when agent picked | `functions.empty*` | header `StatusLine` |
| Connectors | first `GET /api/connectors` | `connectors.empty` | header `StatusLine` |
| Events | first `GET /api/events` | `events.empty` | header `StatusLine` |

## Chat 502

On send failure the user bubble stays. An assistant-side row shows `formatPublicError(e)` from `String(e)` (gateway `502` body, e.g. LLM `401`), truncated, with Bearer / `sk-` / `gsk_` / `xai-` / `AIza` / `wh_` / JSON token fields redacted. The same text is in the header error row — not swallowed. `load`/`send` ignore stale completions when the selected session changes.

i18n: `common.loading` and `common.error` in both `vi.ts` and `en.ts` (`MsgKey`).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD <sibling>/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

New Go endpoints, GET webhook registry, wiring DEMO mocks as live CRM, Chat SSE (053).

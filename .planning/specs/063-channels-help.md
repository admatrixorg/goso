# SPEC 063 — Channels: show env names (no secrets)

> LOCKED: 2026-08-28. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> **No channel tokens in git, UI, or GET body.**

`GET /api/channels` returns `{channels:[{name,configured}], lite}`. `configured` is env-token non-empty (`gateway/internal/channel/catalog.go` `tokenEnv`). UI is a yes/no table. User cannot see **which env** to set. Do **not** add PATCH/sqlite overlay in this spec (parked — like 056 for LLM only).

## HTTP (optional small)

Add public `env` string on each catalog row (the **variable name**, e.g. `GOSO_TELEGRAM_BOT_TOKEN`, never the value). Update tests that compare exact JSON.

If you skip Go, hardcode the same 7 names in the page — must match `tokenEnv` exactly.

## UI

- Table: name, configured, env var name (monospace). Copy button optional.
- Note: channels are env-only; Lite still uses `channels.liteOff`.
- i18n vi+en. StatusLine unchanged.

`docs/qa/063-channels-help.md`. Commit `admatrixmdp/spec063-channels-help`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · `go test ./...` · `go build` if Go changed · agpl-check 0.

## Non-goals

Saving tokens from UI, SQLite overlay, live Discord/Telegram, deleting adapters.

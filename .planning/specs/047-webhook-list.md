# SPEC 047 — GET /api/webhooks list + Webhooks page registry

> LOCKED: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

043 documented **no GET list**. `webhook.Registry.List()` already returns `[]Public{id, token_prefix}` — **no secrets**. Wire HTTP + UI only.

## HTTP

- `GET /api/webhooks` → `{webhooks:[{id, token_prefix}]}` (empty array if none). Never `token` / `hmac_key`.
- Keep `POST /api/webhooks` (secret once) and `POST /api/webhooks/llm`.
- Optional `DELETE /api/webhooks/{id}` if a small `Registry.Delete` is needed for registry CRUD; otherwise GET+POST is enough. Tests: httptest create → GET shows prefix only, no full token.

## UI

`WebhooksPage`: list from GET (StatusLine loading/empty/error). Create button still POST; last-created secret once then redact. Do not invent fields.

i18n vi+en. `npm run typecheck`. `docs/qa/047-webhook-list.md`.

Commit `admatrixmdp/spec047-webhook-list`. Do not merge.

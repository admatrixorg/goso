# QA — SPEC 047 GET /api/webhooks list + Webhooks registry

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. 043 documented **no GET list**; this SPEC wires HTTP + UI only. `webhook.Registry.List()` already returns `[]Public{id, token_prefix}` — **no secrets**.

## What changed

- `GET /api/webhooks` → `{webhooks:[{id, token_prefix}]}`. Empty registry → `webhooks: []` (array, not null). Never `token` / `hmac_key`.
- Keep `POST /api/webhooks` (secret once) and `POST /api/webhooks/llm`. No DELETE.
- Control-plane `WebhooksPage` loads the registry from GET. StatusLine loading / empty / error. Create still POSTs; last-created token/hmac shown once then redacted after copy. No invented fields.
- i18n vi+en (`webhooks.list` `webhooks.empty` `webhooks.noSecrets` `webhooks.lastEmpty`).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Coordinator after merge: restart `:18080` with new `goso-gateway`; Vite `:3000` proxies to `:18080`.

## Proof

- httptest POST create then GET list: prefix matches, body never contains full token or hmac (`TestWebhookAPI_ListPublicOnly`).
- Empty GET is `{webhooks:[]}` not null.
- `TestWebhookAPI_BearerAndHMAC` and `TestRegistry_CreateHashedAndAuth` still cover secret-once create + hashed list.

## Non-goals

DELETE registry, extra webhook fields, MCP `/v1/webhooks`, live demo bind/kill, copying goclaw/ZaloCRM.

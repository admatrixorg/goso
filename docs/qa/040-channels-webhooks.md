# QA — SPEC 040 Channels, webhook API, WS RPC

Date: 2026-08-27. Clean-room. Closes matrix rows **C0–C10, W1–W2** with **fake/inject tests**. Live tokens = **DI-01..07** — not in git; `.env.example` placeholders empty. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No goclaw copy. Tests use `httptest` + injectable Sender, not live Discord/Slack/Feishu/WhatsApp.

## What changed

### Channels (C0–C7)

Keep Telegram / Zalo OA / Personal. Added Discord, Slack, Feishu, WhatsApp adapters (`HandleUpdate` + `Name`). Each parses a **minimal inbound JSON fixture** documented in its `_test.go`. Session keys: `discord:{channel_id}`, `slack:{channel_id}`, `feishu:{chat_id}`, `whatsapp:{from}`. LLM Echo via injectable Sender. Outbound HTTP goes through `httptest` fake servers (no live network).

`GET /api/channels` lists all **7** names as `{name, configured}`. Empty token env → still listed, `configured: false`.

WhatsApp is a **Cloud-API-shaped webhook stub** (`object=whatsapp_business_account` / `entry[].changes[].value.messages[]`). Native protocol vs Business Cloud API = **DI-01** — this SPEC implements Business Cloud shape only.

### Webhook API (W1–W2)

- `POST /api/webhooks` (admin) creates `{id, token, token_prefix, hmac_key}`. Bearer token stored hashed; secrets shown **once**.
- `POST /api/webhooks/llm` with `Authorization: Bearer wh_…` **or** `X-Goso-Signature: t=unix,v1=hex` HMAC-SHA256 over `t.body` (timestamp + `.` + raw body). Bypasses `GOSO_ADMIN_TOKEN` (own credential).
- Body `{input, mode: sync|async, session_id?}`. Sync: Chat Echo 200. Async: **202** + `id`.
- Tests: valid bearer, valid HMAC, bad HMAC → **401**.

Header is `X-Goso-Signature` (goso-shaped, not a copy of GoClaw).

### WS RPC (C8, C9 allowlist)

`GET /ws` is JSON `{op, payload}`: `ping`→`pong`, `chat` `{session_id, message}`→ reply text (Echo). Not the old `"echo: "+raw` prefix. Empty `GOSO_WS_ORIGINS` keeps previous allow-all origin check; if set (comma-separated), Origin must match.

C9 pairing codes are still later; origin allowlist is the allowlist shipped here. C10 MCP `/v1/channels` is unchanged (goso-mcp still talks GoClaw-shaped `/v1`); gateway list is `GET /api/channels`.

## Commands

```
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Seven names + `configured:false` when env empty (`TestCatalog_SevenNamesUnconfigured`, `TestChannelsAPI_ListsSeven`, `TestChannelsListsSeven`).
- Discord/Slack/Feishu/WhatsApp inbound fixtures + Echo Sender + session labels (`TestDiscord_HandleUpdate`, `TestSlack_HandleUpdate`, `TestFeishu_HandleUpdate`, `TestWhatsApp_CloudAPIHandleUpdate`).
- Outbound mock HTTP (`TestDiscord_SendHttptest`, `TestWhatsApp_SendHttptest`).
- Webhook bearer / HMAC / bad HMAC 401 / async 202 (`TestWebhookAPI_BearerAndHMAC`, `TestRegistry_CreateHashedAndAuth`).
- LLM webhook bypasses admin Bearer (`TestWebhookLLMBypassesAdmin`).
- WS ping/pong + chat reply not echo-prefix (`TestWS_PingPongAndChat`, `TestWS_NotEchoOnlyPlainText`).
- Origin allowlist (`TestWS_OriginAllowlist`).

## Non-goals

Live Discord/Slack/Feishu/WhatsApp apps, CRM Settings outbound webhooks (033 CẮT), copying goclaw channel code, pairing codes, MCP `/v1/channels` toggle, native WhatsApp stack (DI-01).

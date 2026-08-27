# SPEC 040 — Channels, webhook API, WS RPC

> LOCKED: 2026-08-27. Closes **C0–C10, W1–W2** with **fake/inject tests**. Tokens = DI-01..07 — **do not** invent production tokens. `.env.example` placeholder names only.

## Channels

Keep Telegram / Zalo OA / Personal. Add adapters: **Discord, Slack, Feishu, WhatsApp** implementing the same channel interface (`HandleUpdate` + `Name`).

Each new adapter:

- Parse a **minimal** inbound JSON fixture (documented in test).
- Session key `discord:{channel_id}` etc.
- Call LLM Echo in tests via injectable Sender.
- Outbound send is a mock HTTP client — **no live network**.

`GET /api/channels` lists all **7** names. Disabled if token env empty (still listed, `configured: false` OK).

WhatsApp: **Cloud-API-shaped webhook stub** (not native stack). QA: “native vs Business = DI-01”.

## Webhook API (goso-shaped, not copy)

- `POST /api/webhooks` admin creates `{id, token_prefix, hmac_key}` stored hashed; response shows secret **once**.
- `POST /api/webhooks/llm` with `Authorization: Bearer wh_…` **or** `X-Goso-Signature: t=unix,v1=hex` HMAC-SHA256 over `t.body`.
- Body `{input, mode: sync|async, session_id?}`. Sync: run Chat Echo; async: 202 + id.
- Tests: valid bearer, valid HMAC, bad HMAC → 401.

## WS RPC

Replace `ws.go` echo-only prefix with JSON messages `{op, payload}`: `ping`→`pong`, `chat` `{session_id, message}`→ reply text. Origin: if `GOSO_WS_ORIGINS` empty, keep current (document); if set, allowlist. Tests with `httptest` + gorilla/nhooyr already in module.

## Non-goals

Live Discord/Slack apps, CRM settings webhooks (033 CẮT unless Dat un-cuts), copying goclaw channel code.

## QC

`go test ./...`, build, agpl 0, `docs/qa/040-channels-webhooks.md`. Commit, do not merge.

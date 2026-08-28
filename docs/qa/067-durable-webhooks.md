# QA — SPEC 067 Durable webhooks

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Closes **CTO-04**.

goso-shaped names only: `/api/webhooks`, `X-Goso-Signature`, `X-Goso-Delivery-Id`. No `X-GoClaw-*`, no new `/v1/webhooks` surface (existing `/api` → `/v1` alias for GET list/get/job stays).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored.

| Behavior | Cite |
|----------|------|
| Inbound LLM webhook Bearer **or** HMAC; HMAC `t=<unix>,v1=<hex>` over `{ts}.{body}` | `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` § Webhook API; `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §3 |
| Timestamp skew **300s**; replay nonce `sha256(tenant\|sig)` TTL **320s** → 401 `hmac_replay` | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §3 HMAC Replay Protection |
| Admin CRUD create/list/get/patch/rotate/revoke; secret shown **once** | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §2 |
| Sync LLM 200 (30s) / async 202 `{call_id,status:queued}` | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §4 |
| `Idempotency-Key` 24h; same key+body cached; same key different body **409** | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §6 |
| Outbound callback at-least-once; HMAC sign; delivery-id stable across retries | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §7; `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/00-architecture-overview.md` §12 Webhook Subsystem |
| Retry `[30s, 2m, 10m, 1h, 6h]` ±10% jitter; 2xx done; 4xx (not 429) dead; 5xx/net retry; 429 Retry-After cap 6h | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §7 Retry Schedule; architecture worker/backoff |
| Persist `webhooks` + `webhook_calls`; secret encrypted at rest AES-256-GCM | architecture §12 Secret Encryption; `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/webhooks.md` §14 |
| SSRF on `callback_url` at delivery | architecture outbound flow |

goso mapping (self-written): HMAC header is `X-Goso-Signature`; outbound delivery id is `X-Goso-Delivery-Id`; jobs table is `webhook_jobs` (not `webhook_calls`); replay nonce is `sha256(webhook_id + "|" + v1)` (no tenant_id — SPEC 071). HMAC signing key bytes stay **hex ASCII as today** (`Sign` test vector unchanged).

## What changed

- SQLite `webhooks` + `webhook_jobs` (`CREATE IF NOT EXISTS`) and matching memory store. Registry is store-backed.
- HMAC material: AES-256-GCM via `GOSO_MASTER_KEY` into `hmac_enc` when the key is set. Demo / empty master key is **hashed-only** (Bearer `token_hash` persists; HMAC key stays in-process and is not written plaintext). Create does **not** 503 without a master key so `GOSO_ENV=demo` still works.
- Freshness `|now-t| > 300s` → 401. Replay nonce 320s in-process → 401. Rotate invalidates the old bearer immediately. DELETE revokes (401 after).
- `POST /api/webhooks/llm` sync 200 `{id,reply,session_id}` / async 202 `{id,status:queued}`. `GET /api/webhooks/jobs/{id}` (admin). Optional `callback_url` worker with retries; tests set `GOSO_WEBHOOK_RETRY_MS`.
- `Idempotency-Key` 24h: same key+body cached; same key different body 409.
- `security.CheckURL` on `callback_url` at enqueue and delivery when SSRF/production is on. Demo (`GOSO_ENV=demo`, SSRF unset) still allows httptest loopback.
- Outbound POST `{id,status,reply,error}` with `X-Goso-Delivery-Id`, `X-Goso-Signature`, `User-Agent: goso-webhook/1`.
- Control-plane `WebhooksPage`: rotate, revoke, `require_hmac`, i18n vi+en, StatusLine, secret-once on create and rotate.

Nonce cache is single-node (accepted).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 068.

## Proof

- Persist across SQLite reopen: Bearer still authenticates; GET list still shows id (`TestWebhookAPI_PersistSQLiteReopen`, `TestRegistry_PersistAcrossReopen`, `TestSQLiteStore_WebhookPersist`).
- HMAC `t` older than 301s → 401; `t=now` → 200 (`TestWebhookAPI_StaleHMAC401`, `TestRegistry_StaleHMAC`).
- Same valid HMAC twice within window → second 401 (`TestWebhookAPI_ReplayHMAC401`, `TestRegistry_ReplayHMAC`).
- Async 202 then `GET /api/webhooks/jobs/{id}` becomes `done` with Echo (`TestWebhookAPI_AsyncJobGETDone`).
- Callback httptest: 2xx → done; 500 then success on retry (`GOSO_WEBHOOK_RETRY_MS`); 400 → dead with 1 attempt (`TestWebhookAPI_Callback2xxDone`, `TestWebhookAPI_Callback500ThenRetry`, `TestWebhookAPI_Callback400Dead`).
- Idempotency-Key replay same body returns first status; different body 409 (`TestWebhookAPI_Idempotency`).
- Rotate invalidates old bearer (`TestWebhookAPI_RotateInvalidatesBearer`, `TestRegistry_RotateInvalidatesBearer`).
- Existing `TestWebhookAPI_BearerAndHMAC` still green (fresh timestamps).
- Encrypted `hmac_enc` when `GOSO_MASTER_KEY` is set; HMAC still verifies after reopen (`TestRegistry_HMACEncryptedPersists`).

## Non-goals

Tenant_id (071). Channel `message` kind depth. Live vendor callbacks. Copying goclaw header names. Binding demo ports. Merge. SPEC 068.

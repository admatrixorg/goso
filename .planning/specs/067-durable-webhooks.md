# SPEC 067 — Durable webhooks (persist, freshness, replay, async)

> LOCKED after SPEC 066 merge. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> goso-shaped names stay: `/api/webhooks`, `X-Goso-Signature`. Do **not** introduce `X-GoClaw-*` or `/v1/webhooks`.

Closes **CTO-04**. Audit: `docs/qa/audit-cto-2026-08-28.md`. Matrix W1/W2 currently over-claimed CÓ in 054 — this SPEC makes them honestly durable.

## 0. GoClaw behavior (READ-ONLY cite — do not copy code)

Source: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC).

| Behavior | Cite |
|----------|------|
| Inbound LLM webhook Bearer **or** HMAC; HMAC `t=<unix>,v1=<hex>` over `{ts}.{body}` | README.md § Webhook API; `docs/webhooks.md` §3 |
| Timestamp skew **300s**; replay nonce `sha256(tenant\|sig)` TTL **320s** → 401 `hmac_replay` | `docs/webhooks.md` §3 HMAC Replay Protection |
| Admin CRUD create/list/get/patch/rotate/revoke; secret shown **once** | `docs/webhooks.md` §2 |
| Sync LLM 200 (30s) / async 202 `{call_id,status:queued}` | `docs/webhooks.md` §4 |
| `Idempotency-Key` 24h; same key+body cached; same key different body **409** | `docs/webhooks.md` §6 |
| Outbound callback at-least-once; HMAC sign; delivery-id stable across retries | `docs/webhooks.md` §7; `docs/00-architecture-overview.md` §12 |
| Retry `[30s, 2m, 10m, 1h, 6h]` ±10% jitter; 2xx done; 4xx (not 429) dead; 5xx/net retry; 429 Retry-After cap 6h | `docs/webhooks.md` §7 Retry Schedule; architecture worker/backoff |
| Persist `webhooks` + `webhook_calls`; secret encrypted at rest AES-256-GCM | architecture §12 Secret Encryption; `docs/webhooks.md` §14 |
| SSRF on `callback_url` at delivery | architecture outbound flow |

Do **not** read or paste goclaw `.go` sources. Behavior only.

## 1. goso today (evidence)

- In-memory `gateway/internal/webhook/registry.go`: hashed bearer, HMAC over `t.body`, jobs map. Lost on restart.
- HMAC **parses** `t=` but **no** freshness, **no** replay nonce.
- `POST /api/webhooks/llm` async: 202 + goroutine; `GetJob` exists, **no HTTP GET**.
- No `callback_url`, no persist, HMAC key held in process memory (not `secrets` table).
- CP `WebhooksPage.tsx`: create + list prefix + secret-once. No rotate/revoke/job status.

## 2. goso behavior (self-written)

### Persist

SQLite tables in `gateway/internal/store/sqlite.go` (CREATE IF NOT EXISTS + ALTER-safe):

- `webhooks(id, name, kind, agent_id, token_prefix, token_hash, hmac_enc, require_hmac, revoked, created_at)`
- `webhook_jobs(id, webhook_id, status, input, reply, error, callback_url, attempts, next_attempt_at, idempotency_key, body_hash, lease_token, created_at, updated_at)`

Statuses: `queued | running | done | failed | dead`.

Encrypt `hmac_enc` with existing AES-256-GCM `GOSO_MASTER_KEY` / `secrets` helper (same as LLM keys). If master key missing in demo, fail closed on **create** with 503 public error, or store-hashed-only path documented in QA — prefer encrypt when key present.

Registry becomes a store-backed service. Memory store must still work in unit tests (interface).

### Auth

Keep `X-Goso-Signature: t=<unix>,v1=<hex>` HMAC-SHA256 over `ts + "." + raw body` with the **hex key bytes as today** (do not silently change the existing test vector). Constant-time compare.

New:

1. **Freshness:** `|now-unix(t)| > 300s` → 401.
2. **Replay:** after valid HMAC, remember `sha256(webhook_id + "|" + v1)` for **320s**. Duplicate → 401. In-memory nonce cache OK (single-node, document).
3. **Revoked** row → 401.
4. `require_hmac=true` rejects Bearer.

### Admin API (existing `/api/webhooks` +)

- `POST /api/webhooks` — optional JSON `{name, kind, agent_id, require_hmac}`. Default kind `llm`. Secret + hmac_key returned **once**.
- `GET /api/webhooks` — public rows only (no secrets).
- `GET /api/webhooks/{id}` — public row.
- `POST /api/webhooks/{id}/rotate` — new secret; old invalid immediately.
- `DELETE /api/webhooks/{id}` — revoke.
- `GET /api/webhooks/jobs/{id}` — job status (admin token). Async callers may use this; webhook bearer **not** required if admin, or allow webhook auth on this GET.

### LLM inbound

`POST /api/webhooks/llm` body `{input, mode, session_id, callback_url}`.

- sync: run chat (existing pipeline/provider after 066 resolve if session has agent), 200 `{id, reply, session_id}`.
- async: persist job `queued`, 202 `{id, status}`. Optional `callback_url` (https or http-httptest; production SSRF via `security.CheckURL` when `GOSO_SSRF=1` / production).
- `Idempotency-Key`: same key + same body_hash → return cached status/body; same key different hash → 409.

### Outbound worker

In-process poller (start from `serve.New`). Claim job with lease_token CAS. Run chat. If `callback_url` set: POST JSON `{id, status, reply, error}` with:

```
X-Goso-Delivery-Id: <job id>
X-Goso-Signature: t=...,v1=...
Content-Type: application/json
User-Agent: goso-webhook/1
```

Retry delays default **[30s, 2m, 10m, 1h, 6h]**; tests override via `GOSO_WEBHOOK_RETRY_MS=10,20,30` (or constructor). 2xx → done. 4xx except 429 → dead. 5xx/net → retry. After schedule exhausted → `dead`.

Do not fire callbacks to loopback when SSRF on, except tests that disable SSRF or inject CheckURL hook.

### Control plane

`WebhooksPage`: rotate, revoke, show `require_hmac`, optional last-job id/status if create-async later. i18n vi+en. StatusLine. typecheck.

Keep secret-once / copy-then-redact behavior.

## 3. Tests (mandatory)

1. Create → restart registry from same SQLite file → Bearer still authenticates; GET list still shows id.
2. HMAC with `t` older than 301s → 401; `t=now` → 200.
3. Same valid HMAC twice within window → second 401.
4. Async 202 then `GET /api/webhooks/jobs/{id}` becomes `done` (Echo provider).
5. Async + callback_url httptest: 2xx marks done; 500 then success on retry (short retry env); 400 marks dead with 1 attempt.
6. Idempotency-Key replay same body returns first status; different body 409.
7. Rotate invalidates old bearer.
8. Existing `TestWebhookAPI_BearerAndHMAC` still green (fresh timestamps).

## 4. Non-goals

Tenant_id (071). Channel `message` kind depth. Live vendor callbacks. Copying goclaw header names. Binding demo ports.

## 5. QC

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

`docs/qa/067-durable-webhooks.md` must contain the goclaw cite table (paths only, no pasted source). Commit `admatrixmdp/spec067-durable-webhooks`. Do not merge. Do not start 068.

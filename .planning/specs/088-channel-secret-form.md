# SPEC 088 — Control-plane Channels write-only secrets

> After 087. Clean-room Go/React. Do not copy GoClaw / ZaloCRM. Demos `:8082` `:8091` must not be bound or killed. `:18080` / `:3000` may restart.

UX notes (behavior cite, no secrets): `docs/qa/088-goclaw-channel-ux-notes.md`.

## Goal

Operator can paste a **Telegram bot token** (and OA access token + app secret) on Control Plane Channels, persist it in the 084 secrets-box, and see connect/health — without GET ever returning plaintext.

## Behavior (learned, not copied)

GoClaw dashboard: password Bot Token on Create; later Credentials tab empty + “leave blank to keep”; “Encrypted server-side. Never returned in API responses.” Providers list “API key set”. Channel DM pairing ≠ dashboard pairing.

## Plan

1. `PUT /api/channels/{name}/secrets` — write-only string fields. Empty = keep. Requires `GOSO_MASTER_KEY`. Keys `channel:telegram:bot`, `channel:zalo-oa:access`, `channel:zalo-oa:secret`. Env wins at resolve time. Phase-2 → 409. Personal → 400 (QR, not token form). `PATCH` still 400 on token keys.
2. `GET /api/channels` — `secret_set`, `from_env`, `writable[]`. Never `token` / `bot_token` / plaintext.
3. `POST /api/channels/{name}/test` — Telegram `getMe` (redact token from errors); OA = both secrets present; Personal = QR/`secret_set`; parked 409.
4. Catalog: telegram/OA `configured` if env **or** box. Resolve box in Telegram/OA Start and send.
5. CP: password fields, never filled from GET, clear after save, env-wins banner, Test/Connect, i18n vi+en.

## Tests

PUT → GET `secret_set` without leak; PATCH token still 400; no master key 503; env wins; Telegram test against httptest `getMe`.

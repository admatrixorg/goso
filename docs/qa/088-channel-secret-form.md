# QA — SPEC 088 Channel write-only secrets

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. Demos `:8082` `:8091` not bound or killed.

UX notes: `docs/qa/088-goclaw-channel-ux-notes.md`.

## Proof

- `PUT /api/channels/{name}/secrets` writes `channel:<name>:<kind>` when `GOSO_MASTER_KEY` set; empty field 400; no master key 503; PATCH token still 400; discord 409; personal 400 (QR).
- `GET /api/channels` `secret_set` / `from_env` / `writable`; plaintext token absent.
- Env wins over box (`TestPutChannelSecrets_EnvWinsFlag`).
- `POST /api/channels/telegram/test` getMe against httptest; discord test 409.
- Catalog box makes telegram `configured` without leaking.
- CP typecheck. agpl 0.

## Click (after gateway + CP refresh)

`http://127.0.0.1:3000` → Channels:

1. Telegram row shows password **Bot token** + Save secrets + Connect / test.
2. Field empty on load (no stored value).
3. OA shows access token + app secret.
4. Personal has QR copy, no bot-token field.
5. discord/slack/feishu/whatsapp: parked copy, no form.
6. GET network tab: no `bot_token` value.

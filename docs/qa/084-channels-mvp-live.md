# QA — SPEC 084 Channels MVP Live

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not invent production tokens. Do not merge.

LOCKED SPEC: `.planning/specs/084-channels-mvp-live.md` §10 (17 answers + 2 extras).

## GoClaw behavior (READ-ONLY cite — paths only)

| Behavior | Cite |
|----------|------|
| Channel policy / pairing / bindings | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/05-channels-messaging.md` |
| Secrets not in config JSON | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/00-architecture-overview.md` |
| Device pairing ≠ channel DM pairing | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/20-api-keys-auth.md` |

goso mapping (self-written): Telegram `GOSO_TELEGRAM_MODE=poll|webhook` default poll; webhook needs `GOSO_PUBLIC_URL`. OA webhook-only, required `GOSO_ZALO_OA_ACCESS_TOKEN` + `GOSO_ZALO_OA_SECRET`. Personal = QR surface + sidecar inject (no in-process protocol). WS `ws_up` on `/healthz` and `/api/stats`, not a catalog row. Slack `env_names` bot+app, health `parked`. Secrets box `channel:<name>:<kind>` when `GOSO_MASTER_KEY` set; env wins. Lite forbids Start.

## Proof

- Store: `TestChannelConfig_PutGet`, `TestChannelPairing_PendingCap`, `TestDeleteSecret`
- Policy/pairing: `TestPolicy_*`, `TestPairing_*`, `TestChannelPairing_HTTP*`
- PATCH: `TestPatchChannel_NonSecretBinding`, token 400 kept
- Catalog: 7 names, Slack 2 env names, OA 3 help names, phase-2 parked, Lite no Start
- Telegram: getMe httptest, webhook without public URL failed, `X-Goso-Telegram-Secret`
- OA verify demo/prod; Personal QR no cookie; logout box only
- `ws_up` on stats
- CP typecheck
- Live smoke script skip-always without flags

## Live smoke

`scripts/e2e-channels-live.sh` exits 0 if flags/tokens missing. Not part of `make verify`. CI must not set `GOSO_LIVE_*`.

## Non-goals

Live Discord/Slack/Feishu/WhatsApp Start. In-process Zalo Personal protocol. Native WhatsApp (DI-01). Merge.

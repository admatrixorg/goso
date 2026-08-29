# QA — SPEC 094 Channels operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live Discord/Slack. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Channels: instance health, agent bind, pairing policy, group overrides, test/connect/logout, write-only credentials never returned on GET | `docs/qa/090-goclaw-sidebar-ux.md` Channels row |

goso mapping (self-written): live tab `channels` in [App.tsx](../../control-plane/src/App.tsx) still renders [ChannelsPage](../../control-plane/src/pages/ChannelsPage.tsx). Pairing Approve/Deny stays the first card (089). Catalog is the fixed seven names; this SPEC does not invent a create-instance dialog.

Out of scope: live Discord/Slack, Nodes (105), Workstations (106), copying GoClaw create-instance chrome.

## What changed

- Pairing panel remains first. Telegram and Zalo OA `dm_policy` PATCH selects stay on that panel. Pending rows show channel, sender, expires, Approve/Deny — no code re-entry (`sanitizePairingItem` drops `code` / `code_hash`).
- Per catalog row: health badge, redacted `last_error`, remediation copy, agent bind, write-only secret fields, Connect/test. Telegram `bot_token`; OA `access_token` + `app_secret`; Personal QR + logout (no bot token). Phase-2 discord/slack/feishu/whatsapp stay parked.
- Phase-1 rows also PATCH `group_policy`, `require_mention`, and `allow_from`. Empty PUT `/api/channels/{name}/secrets` stays **400**. New `DELETE /api/channels/{name}/secrets` clears boxed secrets (`channel:<name>:<kind>`). Env still wins after clear. GET never returns plaintext (`secret_set` / `from_env` / `writable` only). Personal DELETE 400; parked DELETE 409.
- Search/health filter, loading/empty/filter-empty/error. i18n vi+en. CP typecheck. Tests for helpers. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/httpapi ./gateway/internal/channel ./gateway/internal/store ./gateway/internal/secrets -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `filterChannels`, `secretPutBody` (blank fields omitted), `parseAllowFrom`, `channelRemediation`, `canClearBox`, `sanitizePairingItem` (no code).
- `go test` httpapi: empty PUT remains 400; DELETE telegram/OA clears box and GET has no plaintext; env-wins after DELETE; discord 409; personal 400.
- GET `/api/channels` still omits `bot_token` / `access_token` / `app_secret` keys. Password inputs stay empty on load.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Live Discord/Slack. Nodes (105). Workstations (106). Copying GoClaw dialogs. Binding/killing demo ports. Merge. Inventing live vendor tokens.

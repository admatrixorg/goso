# SPEC 094 — Channels operator surface

> After 093. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 094. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Channels.

## Goal

Consolidate **ChannelsPage**: instance create/bind, health diagnosis, pairing policy, group overrides, test/connect/logout, remediation. Every credential write-only (`secret_set`, never-return GET, rotate/clear). Keep pairing Approve/Deny (089) at top. Do not copy GoClaw dialogs.

## AC

- [ ] Pairing panel stays first. Policy PATCH (`dm_policy`) remains. Pending: channel, sender, expires, Approve/Deny — no code re-entry.
- [ ] Per-channel: health, last_error (redacted), bind agent, write-only secrets, test/connect. Telegram bot token + OA access/app secret; Personal QR (no bot token). Phase-2 parked.
- [ ] Explicit rotate/clear where a secret is set (empty PUT already 400 — add a clear/delete-secret path or documented “unset box” if store supports it). GET never returns plaintext.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/094-channels-operator.md`.

## Out of scope

Live Discord/Slack. Nodes/Workstations pages (105/106). Copying GoClaw create-instance dialog chrome.

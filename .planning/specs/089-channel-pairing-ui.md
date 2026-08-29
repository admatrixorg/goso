# SPEC 089 — Channel Pairing panel first on Channels

> After 088. Clean-room React/Go. Do not copy GoClaw. Do not bind/kill `:8082` `:8091`.

User report (correct): scrolling Channels never shows a place to approve pairing. The card existed **below the catalog** (`top≈1140` vs viewport `939`) with EmptyState “Không có yêu cầu ghép.” — looks like the feature is missing.

## UI

1. Pairing panel **first** on Channels (above catalog).
2. Title: **Duyệt mã ghép Telegram/Zalo**.
3. Empty: short guide (user chats bot → 8-char code → request appears here → Approve/Deny). Not a dead EmptyState.
4. Pending: `channel` + `sender_id` + `expires_at` + Approve/Deny. **No code re-entry** (084 hashed).
5. Show effective Telegram `dm_policy`. Demo default `open` (084) does not mint pairing rows — button to PATCH `dm_policy=pairing`.
6. i18n vi+en.

## Backend verify (084)

Inbound Telegram with `dm_policy=pairing` must persist a pending row visible on `GET /api/channel-pairing` (no plaintext code). If missing, fix ingest. Overlay GET catalog with merged default policy so the row is not `—`.

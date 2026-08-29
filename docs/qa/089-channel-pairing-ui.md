# QA — SPEC 089 Channel Pairing panel

Date: 2026-08-29. Clean-room. Demos `:8082` `:8091` not killed.

## Before (browser, 127.0.0.1:3000 Channels)

- Card “Ghép kênh đang chờ” existed but `getBoundingClientRect.top ≈ 1140` vs viewport `939` → **not in view**.
- Copy: EmptyState “Không có yêu cầu ghép.” + meta `0`.
- `GET /api/channel-pairing` → `{"items":[]}`.
- Telegram `dm_policy` omitted (`—`). Gateway `GOSO_ENV=demo` → default DM **open** (084), so inbound does not mint pairing rows.

## Backend (084 verify)

- `TestTelegram_PairingSendsCode` now asserts a **pending** store row (hashed, no plaintext on GET).
- `TestTelegramWebhook_CreatesPendingOnGET`: production env, POST webhook → `GET /api/channel-pairing` one pending `telegram` / sender `777`, no `code` / `code_hash`.
- Overlay: demo GET telegram `dm_policy=open`, OA `pairing`.

## After UI

Pairing panel first. Title “Duyệt mã ghép Telegram/Zalo”. Guide + policy + enable pairing when open. Pending: channel, sender, expires, Approve/Deny. No code field.

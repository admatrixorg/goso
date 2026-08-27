# SPEC 055 — Refresh parity matrix + optional router9 e2e + Lite channel hide

> LOCKED: 2026-08-28. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`. **Last queue item.**

## 1. Matrix 034

Add `docs/qa/034-goclaw-parity-update-054.md` (do **not** rewrite the original snapshot in place — keep historical). Table: row id | original status | **now** after 035–054 | evidence path. Mark closed rows CÓ/PARTIAL honestly. Parked DI stay parked.

## 2. Optional e2e router9

`scripts/e2e-router9.sh`: if `GOSO_ROUTER9_BASE_URL` unset or `curl /v1/models` fails → **skip 0**. If up: POST /api/chat with admin token + model default, assert HTTP 200 and non-empty reply. No secrets in script. Document in SETUP/QA.

## 3. Lite channel hide

`GOSO_LITE=1`: Control-plane Channels page shows a one-line “Lite: channels off” empty (still listed via API is OK **or** GET `/api/channels` returns `configured: false` only). Prefer **UI hide/note** over deleting adapters. Tests: i18n + lite env if Go change.

`docs/qa/055-matrix-e2e-lite.md`. Commit `admatrixmdp/spec055-matrix-e2e`. Do not merge.

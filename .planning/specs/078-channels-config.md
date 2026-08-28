# SPEC 078 — Channel config depth (no live tokens)

> After 077. **Do not fill secrets.** DI-01..07 stay parked.

Closes **C1–C7** UI/config completeness (adapters already 040).

## GoClaw cite

`docs/05-channels-messaging.md` — per-channel config fields (names only).

## goso plan

1. `GET /api/channels` already lists 7 names + `configured`. Add `env_names[]` help (063 started this) + `missing: true` when env empty. PATCH is **not** allowed to write tokens into sqlite; only `enabled` bool if missing.
2. CP Channels: per-adapter required env list, configured badge, no secret inputs. i18n.
3. Tests: seven names; empty env configured=false; no token in JSON.

Commit `admatrixmdp/spec078-channels-config`.

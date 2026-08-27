# SPEC 050 — Filesystem tools in GOSO_WORKSPACE jail

> LOCKED: 2026-08-27. Clean-room Go. No copy from goclaw-source or ZaloCRM. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

K1 THIẾU: no `read_file` / `write_file` in gateway. `GOSO_WORKSPACE` write jail already exists (041).

## Behavior

- Builtins `read_file` `{path}` and `write_file` `{path, content}` (`connector: "builtin"`).
- **Fail-closed** when `GOSO_WORKSPACE` is empty: `not_configured`, no FS access.
- When set: resolve path inside workspace only. Reject `..`, absolute paths outside jail, symlink escape. Cap read 1MiB. Write creates parent dirs only inside jail. **No exec**, no delete unless you add `delete_file` (optional; skip if timeboxed).
- Advertise on `GET /api/agents/{id}/tools`. `requires_approval: true` for write.
- Tests: temp workspace; empty env fail-closed; escape rejected; round-trip write/read.

## UI

Functions page already lists builtins — write should show approval badge. No new page required unless a tiny “workspace configured” note. i18n if new strings. typecheck.

`docs/qa/050-fs-tools.md`. Commit `admatrixmdp/spec050-fs-tools`. Do not merge.

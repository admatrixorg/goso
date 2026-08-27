# SPEC 049 — Skills loader + use_skill (fail-closed)

> LOCKED: 2026-08-27. Clean-room Go. No copy from goclaw-source or ZaloCRM. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

K6 THIẾU: MCP talks `/v1/skills`; gateway has no skill loader.

## Behavior

- Env `GOSO_SKILLS_DIR`. Empty/unset → skills **not_configured** (no FS walk).
- When set: scan **one level** of subdirs for `SKILL.md` (name = folder). Reject `..` / absolute path / escape from dir (`GOSO_WORKSPACE` jail if set, else the skills dir).
- Builtin tool `use_skill` `{name}`: returns markdown body (cap 64KiB) or `not_configured` / `not_found`. **No exec** of skill scripts.
- `GET /api/skills` `{skills:[{name, path}]}` names only, no full body required (optional `?name=` get). Never list files outside dir.
- Advertise `use_skill` on Functions/builtins list when catalog includes it (`connector: "builtin"`).
- Tests: temp dir with one SKILL.md; empty env fail-closed; path escape rejected.

## UI

Functions page already lists builtins. If GET /api/skills exists, optional small list on Functions or a Skills card — keep matching StatusLine. i18n vi+en. typecheck.

`docs/qa/049-skills.md`. Commit `admatrixmdp/spec049-skills`. Do not merge.

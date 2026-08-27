# SPEC 054 — Cron / heartbeat tools (fail-closed)

> LOCKED: 2026-08-28. Clean-room Go. No copy from goclaw-source or ZaloCRM. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

K8 THIẾU: MCP cron hits `/v1`; gateway has no schedule.

## Behavior

- SQLite table `cron_jobs(id, spec, session_id, message, enabled, last_run)`. `spec` is 5-field cron **or** interval `every:1h` (keep parser small: support `every:Nm|Nh` plus optional 5-field if cheap; if 5-field is too large, **interval only** and document).
- `GET /api/cron` `{jobs:[]}` `POST /api/cron` `{spec, session_id, message}` `DELETE /api/cron/{id}`. Auth same as /api.
- Runner: in-process ticker **1m**; when due, POST equivalent of chat into that session. Fail-closed: no jobs → no-op. Cap 20 jobs. Do not spawn OS cron.
- Builtin tool `cron_list` / skip if HTTP is enough. Tests: create interval job, List, Delete; empty list; invalid spec 400.

## UI

Optional small Functions/Settings card listing jobs + create. Prefer HTTP + a tiny Cron card on Functions if timeboxed; HTTP-only is OK with QA note. i18n if UI. typecheck.

`docs/qa/054-cron.md`. Commit `admatrixmdp/spec054-cron`. Do not merge.

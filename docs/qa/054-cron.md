# QA — SPEC 054 Cron / heartbeat jobs

Date: 2026-08-28. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

K8 was MCP cron tools hitting `/v1` while the gateway had no schedule. This spec adds SQLite jobs, `/api/cron`, and an in-process 1-minute ticker.

## What changed

- SQLite (and memory store) `cron_jobs(id, spec, session_id, message, enabled, last_run)`. Cap **20**.
- `GET /api/cron` `{jobs:[]}` · `POST /api/cron` `{spec, session_id, message}` · `DELETE /api/cron/{id}`. Auth same as `/api`. Same handlers aliased at `/v1/cron` now that `/api` exists (052 no-invent no longer applies to this path).
- Spec parser: interval `every:Nm|Nh` (n≥1) plus optional 5-field cron (`*`, decimal, `*/n` only). Invalid spec → 400.
- Runner: in-process **1m** ticker in `serve.Mux` (skipped under `go test`). Empty job list is a no-op. When due, posts the job message into that session (same path as chat). Failed fires are **not** marked `last_run` and retry next tick. Each fire has a 45s timeout. Disabled jobs skip. 5-field matching is UTC. No OS crontab.
- Builtin `cron_list` skipped — HTTP is enough. MCP tool shapes (`name`/`expression`/`agent_id`, extra PUT/PATCH/run) were not rewritten.
- Control-plane Functions: tiny Cron card (list + create + delete, vi+en).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Empty `GET /api/cron` is `{jobs:[]}` (`TestCron_EmptyList`).
- Create interval job, list, delete, 404 on missing delete (`TestCron_CreateListDelete`).
- Invalid spec 400 (`TestCron_InvalidSpec400`). Five-field `0 * * * *` 201 (`TestCron_FiveFieldAccepted`).
- Cap 21st POST 400 (`TestCron_Cap20`).
- `GET /v1/cron` matches `/api/cron` (`TestCron_V1Alias`, `TestV1AliasesMatchAPI`).
- Ticker: empty no-op; due interval fires and marks `last_run`; disabled skip (`TestTick_*`). Failed fire leaves `last_run` nil and retries (`TestTick_FireErrorDoesNotMark`). `Loop` stops on cancel. Fire persists echo chat (`TestCron_TickFiresChat`).
- Memory + SQLite store CRUD/cap/persist (`TestStore_Cron*`, `TestSQLiteStore_CronPersist`).

## Non-goals

OS crontab, MCP tool rewrite, extra PUT/PATCH/run HTTP, builtin `cron_list`, live demo bind/kill, copying goclaw/ZaloCRM.

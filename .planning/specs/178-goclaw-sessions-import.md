# SPEC 178 — Offline session and message import

Status: implemented on this branch. One PR into `main`. Do not merge from this file.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw. It does not write `memories` and it does not run memory extraction.

## Source

GoClaw has no `messages` table. Messages are the `sessions.messages` jsonb array (`jsonb NOT NULL DEFAULT '[]'`). The export is JSONL, one session object per line, with the GoClaw column names. The tool never opens a live database as the source.

## Mapping

The written tenant is `--tenant`. `--source-tenant` is required and is the only export `tenant_id` that is eligible. Every other id is counted and not written.

A local denylist file is required (`--deny-tenants` or `GOSO_IMPORT_DENY_TENANTS`), one tenant id per line. Blocking is the default. A missing, empty, or malformed file stops the process before any write, including a dry-run. If `--tenant` or `--source-tenant` is listed, the run stops the same way. The report has a `denied` count and does not print those names. The file is not committed.

A session id that already exists in another tenant is `id_collision` and is left where it is. A session in the destination tenant that already exists and differs is `native-exists` and is not updated. An identical rerun is `skipped`.

`--rollback` deletes importer-created message ids (`sessionID` plus a digit suffix) and drops a session only when no real message remains. A session that received a message after import stays, with that message. Dry-run is the default. Native rows and other tenants stay.

Sessions with no resolved agent are stored `archived` and the placeholder agent `import-unresolved.<tenant>` is `enabled=0`, so a later chat turn does not call a model. Re-mapping those sessions is a later change.

The SQLite `messages_ai` trigger stays in place. Imported message ids are removed from `memory_fts` in the same transaction as the insert.

| Source | goso |
|---|---|
| `id` | `sessions.id` and `session_import_links.source_id` |
| `session_key` | `session_import_links.source_key`. Also `sessions.label` when `label` is empty |
| `label` | `sessions.label` |
| `agent_id` | `sessions.agent_id` when an agent in `--tenant` has that `id` or `agent_key`. Otherwise the source id is stored on `session_import_links.source_agent_id`, `session_import_links.agent_id` stays empty, and `sessions.agent_id` points at the placeholder agent `import-unresolved.<tenant>` so the existing NOT NULL foreign key still holds |
| `user_id` | `session_import_links.source_user_id`. `contact_id` is set when that id is a live contact in `--tenant`, or when `(channel, user_id)` matches one contact identifier. A contact in another tenant is not used |
| `created_at` | `sessions.created_at` |
| `model`, `provider` | `sessions.model_override`, `sessions.provider_override` |
| `messages[].role` | `messages.role` |
| `messages[].content` | `messages.content`. A string is stored as given. A list keeps text parts in order. Non-text parts are not stored |
| `messages[].created_at`, else `messages[].timestamp` | `messages.created_at` |
| `messages[].model`, `messages[].provider` | `messages.model`, `messages.provider` |
| `messages[].agent_id`, `messages[].mention_agent_id` | stored only when that id or agent key resolves inside `--tenant` |

Message order is the array order. goso lists messages by `created_at`. A timestamp that is not strictly after the previous stored message is kept one nanosecond later so that order matches the array. Every other timestamp is the source instant, in UTC.

Idempotency is `(tenant, source id)` and `(tenant, session_key)`. A session id that already exists in any tenant is left unchanged. The run does not update or delete existing rows.

## Not imported, needs decision

These columns have no home on `sessions` or `messages`. They are not written. `summary` is not copied into `memories` (memory import is a separate design). `channel` is read only as a contact-identifier hint.

- `summary`
- `input_tokens`, `output_tokens`
- `compaction_count`, `memory_flush_compaction_count`, `memory_flush_at`
- `spawned_by`, `spawn_depth`
- `metadata`
- `team_id` (goso sessions have no team column)
- `channel`
- `updated_at` (goso sessions have no updated_at)
- `messages.id`
- `messages.timestamp` when `created_at` is also set
- non-text content parts
- any other key (`sessions.<key>`, `messages.<key>`)

`user_id` is not a column on `sessions`. The source id is kept on `session_import_links` because that is the only place goso can record a contact reference today.

## Command

`--export`, `--tenant`, and `--db` are required. Dry-run is the default and prints per-tenant counts. `--apply` writes. `--apply` together with `--dry-run` is an error. The command opens `--db` only and refuses a non-local Postgres host.

The file is streamed. Sessions are written in batches. One session is not held with the rest of the file. A line over 16MiB is invalid and does not stop the file.

The SQLite `messages_ai` trigger stays in place during `--apply`. Imported message ids are removed from `memory_fts` in the same transaction as the insert. The import does not call `PutMemory`, `SaveSummary`, or the chat pipeline. `POST /api/chat` on an archived session, or on a session whose agent is disabled, returns 409 and does not call a model. Channel webhooks are unchanged: they still store the incoming customer message. Channel handlers select `telegram` and `zalo` agents, not `import-unresolved.*`.

## Out of scope

Live database reads. MQ Capital. pgvector. Memory extraction and auto-write. Relinking a session after a later agent or contact import (a rerun does not update the row). Copying GoClaw code.

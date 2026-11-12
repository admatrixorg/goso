# SPEC 179 — Episodic summaries into memories

Status: implemented on this branch. One PR into `main`. Sample data is Minh An only. This spec does not read a live GoClaw database and does not copy GoClaw. It does not add a column, a table, or pgvector.

The column list is the `\d episodic_summaries` read on 2026-11-12. No column below is assumed.

## Behavior

1. Input is a JSONL file, one `episodic_summaries` object per line. The tool opens only the goso database named by `--db` (local SQLite, or Postgres on localhost / 127.0.0.1 / ::1).
2. `body` is `summary`. When `key_topics` has a non-empty value, one line is added: `Topics: a, b`. `created_at` is stored as UTC RFC3339Nano. These columns are not stored: `l0_abstract`, `embedding`, `source_type`, `source_id`, `turn_count`, `token_count`, `search_vector`, `promoted_at`, `recall_count`, `recall_score`, `last_recalled_at`.
3. A row whose `expires_at` is before the run clock is `expired` and is not written. `expires_at` null, empty, or equal to the clock is not expired. The clock is the current time. `--now` (RFC3339) overrides it. The checked-in fixture is counted against 2026-06-01T00:00:00Z.
4. `episodic_summaries` has no `team_id` and no `custom_scope`. There is no import branch for them. If a JSON object carries a non-empty `team_id` or `custom_scope`, the row is counted as `team/scope` and is not written. The dry-run still prints that counter when it is zero.
5. `customer-private` and `missing-relation-endpoint` are always printed and stay 0 for this table. A non-empty `user_id` is imported, not held back.
6. `customer_key` is `goclaw:<user_id>` for every non-empty `user_id` (trimmed). It is `""` only when `user_id` is empty. This PR does not read `channel_contacts` and does not guess a match. A later command remaps when a full customer map exists.
7. Each kept row is written into an archived session for the pair (goso agent, GoClaw `session_key`). The session id is `gc-sess:` plus the hex sha256 of the UTF-8 bytes `goso agent id`, a newline, and `session_key`. The memory id is `gc-ep:` plus the lowercase source `id`. Inserts are `ON CONFLICT (id) DO NOTHING`. `SaveSummary` overwrites the latest episodic note in a session, so these rows are not placed in a live session.
8. The goso agent must already exist. `--agents` is a JSONL file produced outside this PR (the agents importer is not imported here):

```json
{"source_agent":"<goclaw agent uuid>","dest_agent":"<goso agent id>","dest_tenant":"<goso tenant>"}
```

A source agent with no line, or a destination agent that is missing or in another tenant, is `unmapped`.

9. `--tenants` is the SPEC 152 map, one source tenant per line:

```json
{"source_tenant":"<goclaw tenant uuid>","slug":"<goclaw slug>","dest_tenant":"<goso tenant>"}
```

`mq-capital` is never written. A source id, slug, or destination equal to `mq-capital` (any case) is `mq`, including a row that is also expired. A source tenant that is not in the file is `unmapped`.

10. `--dry-run` writes nothing. For each source tenant and for `episodic_summaries` it prints read, write, already, archived sessions to create, and the skip counters. `read = write + already + skipped`. A second run writes 0 rows and creates 0 sessions.
11. `--remove --tenant <goso tenant>` deletes that tenant's `memories` and `sessions` whose ids start with `gc-`. A `gc-` session that still has a memory whose id does not start with `gc-` is kept, and that memory is kept. Other tenants are not deleted.

## Columns read

`id`, `tenant_id`, `agent_id`, `user_id`, `session_key`, `summary`, `key_topics`, `created_at`, `expires_at`.

`key_topics` in the JSONL file is a JSON array of strings (the export shape of `text[]`).

## Tests

`TestMinhAnEpisodic` on SQLite and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set): dry-run writes nothing and reports sessions to create; expired, team/scope, mq, unmapped, invalid, and `goclaw:<user_id>` are all exercised; an empty `user_id` is the only empty `customer_key`; a second run inserts nothing; tenant A recall does not return tenant B; customer X recall does not return customer Y or any `goclaw:*` note; a `SaveSummary` on a live session does not change imported notes; `SaveSummary` on an archive session changes only the latest note there; removal deletes `gc-` rows and leaves a live note, including one that sits in an archive session. `#44` plan and ceiling tests are unchanged.

## Non-goals

No goso schema change. No pgvector. No vault write. No knowledge-graph write. No copy of GoClaw. No customer names, phones, or emails in the fixture.

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

`--source-tenant` and `--tenant` are required. Rows for any other source id are `unmapped` and are not written.

10. A local denylist file is required (`--denylist` or `GOSO_TENANT_DENYLIST`), one tenant id per line. If the file is missing, empty, or malformed, the command stops before it opens the database, including on `--dry-run`. If the source tenant (id or slug) or `--tenant` is listed, the command writes nothing and the report is only `denied_tenants` (a count). The file's names are not printed.
11. `--customers` is the shared map: `{"customers":[{"user_id","customer_key"}]}` or the same objects as JSONL. A non-empty mapped key is stored. Otherwise a non-empty `user_id` becomes `goclaw:<user_id>`. An empty `user_id` is the only empty key. A line with `channel`, `sender_id`, and `user_id` and no `customer_key` uses `channel:sender_id`.
12. If a memory id already exists and the stored row differs, the row is `native-exists` and is not updated. An identical row is `already`.
13. `--dry-run` writes nothing. The report prints read, write, already, archived sessions to create, skip counters, and `denied_tenants`. It does not print user ids. `read = write + already + skipped`. A second identical run writes 0 rows.
14. `--remove --tenant` deletes that tenant's `gc-ep:` memories and `gc-sess:` sessions (`GLOB` on SQLite, `LIKE` on Postgres). A `gc-doc:` row is left in place. `PruneEpisodic` also leaves `gc-ep:` ids in place.

## Columns read

`id`, `tenant_id`, `agent_id`, `user_id`, `session_key`, `summary`, `key_topics`, `created_at`, `expires_at`.

`key_topics` in the JSONL file is a JSON array of strings (the export shape of `text[]`).

## Tests

`TestMinhAnEpisodic` on SQLite and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set) covers the Minh An fixture, a mapped customer key and a `goclaw:` fallback, `native-exists` after a hand edit, and recall isolation. `TestDeniedTenantWritesNothing` covers a fake denied slug and a fake denied uuid, with zero rows and no names in the report. `TestPruneKeepsImportedEpisodic` runs on the memory store, SQLite, and Postgres: a Minh An note dated 2026-01-03 with id `gc-ep:` survives a prune that still deletes an older native episodic row. `TestDeleteEpisodicGCLeavesOtherPrefix` removes `gc-ep:` / `gc-sess:` and leaves a `gc-doc:` row's count unchanged.

## Non-goals

No goso schema change. No pgvector. No vault write. No knowledge-graph write. No copy of GoClaw. No customer names, phones, or emails in the fixture.

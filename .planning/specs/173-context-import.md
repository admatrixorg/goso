# SPEC 173 — GoClaw context-file import

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

This is item 2 of the GoClaw to Goso import (goso#26, goso#28, goso#29). It writes the durable `context_files` store from SPEC 150 (PR #35, `d918e99`). It does not add a table, a column, or a version history. Sample data is Minh An only. This spec does not copy GoClaw.

The column list is the read-only schema report for GoClaw Postgres: `agent_context_files` and `user_context_files`. The input is an export file. This tool does not connect to that database.

## Number

| Number | Where it is used |
|---|---|
| 150 | On `main`: durable agent and user context files. This importer writes that table. |
| 152 | On `main`: vault import file bodies. The command shape follows that importer. |
| 173 | This spec. |

## Column mapping

GoClaw natural keys, and where each column goes in `context_files`. The GoClaw uuid primary key is not stored. Goso keeps its own id. Unknown JSON fields are ignored.

| GoClaw column | Agent file | User file |
|---|---|---|
| `tenant_id` | Kept only when it is `--source-tenant`. Written as `--tenant`. | Same. |
| `agent_id` | Selects an agent that already exists in `--tenant`. The stored owner is that agent's id. Scope is `agent`. | The row is imported only when this agent is in `--tenant`. It is not part of the stored user key. |
| `user_id` | Not a column of this table. | Stored as the per-customer `user_key`. Scope is `user`. |
| `file_name` | `name` | `name` |
| `content` | `body` | `body` |
| `created_at` | `created_at` | `created_at` |
| `updated_at` | `updated_at` | `updated_at` |

`agent_id` is bound with `--agent-map` (JSON object of GoClaw id to goso `agent_key`). If the id is absent from the map, an agent whose id or `agent_key` equals `agent_id` is used. The map does not create agents.

Goso user files are per customer, not per agent. The prompt loads every file for that `user_key`. Two agents that share `user_id` and `file_name` and the same content store one row. Different content is `scope-conflict`: nothing is written for that pair, and a row already stored is left in place. A file for one customer is never written onto another customer.

`file_name` must already fit the store: one path segment, 1–64 characters, no slash and no `..`. `user_id` must already be an opaque user key (letters, digits, `.`, `_`, `-`; 1–64). Longer GoClaw values are `invalid` and are not truncated. `content` over 32KiB, or an owner past 16 files or 64KiB, is `too-large` and is not truncated. Empty content is `invalid`.

JSON is one object with `agent_context_files` and `user_context_files`, or one array of row objects. JSONL is one row object per line. A row that contains `user_id` is a user file. The sample is `gateway/internal/contextimport/testdata/minh-an.json`.

## Behavior

1. `goso-context-import` takes `--export`, `--source-tenant`, `--tenant`, and `--db`. `--tenant` is the goso tenant id and is required. It is not defaulted to `default`. `--source-tenant` is the export `tenant_id` to keep. A row whose `tenant_id` does not match is `tenant-mismatch` and is not written. `--agent-map` is optional.
2. The default is a dry-run. It reports the same counts and does not call `PutContextFile`. `--apply` writes. `--apply` together with `--dry-run` is an error. `--dry-run=false` without `--apply` still does not write.
3. `--db` is the only database. A Postgres host other than `localhost`, `127.0.0.1`, or `::1` is refused. `GOSO_DATABASE_URL` is not opened. An export larger than 32MiB is refused before decode. The export is a file, not a connection to GoClaw.
4. An agent file is stored only when `agent_id` resolves inside `--tenant`. The same id on an agent in another tenant is `agent-other-tenant` and is not written onto that agent. A missing id is `agent-missing`. A user file uses the same agent check, then stores `user_id` in `--tenant` only. The same user id in another tenant is a different row and is left as it was.
5. Upsert is keyed on the GoClaw natural key. A repeated `(agent_id, file_name)` or `(agent_id, user_id, file_name)` keeps the last row in that export. A later run with the same content and the same clocks does not insert another row and does not change the stored id, `created_at`, or `updated_at`. A different body replaces that same row and stores the source clocks. There is no version table. The HTTP handler still leaves the clocks zero, so a direct PUT keeps today's behavior.
6. The report splits agent rows and user rows by the export `tenant_id`. For each section, `input = inserted + updated + skipped + rejected`. Logs and the report contain file names, agent ids or user ids, and reason codes. They do not contain file bodies.
7. `GOSO_CONTEXT_FILES` stays default off. The importer writes the table either way. Chat does not read it until the flag is on. With the flag on, the pipeline system message contains the block `contextfile.Prompt` returns for that agent and that user. `agents.instructions`, memory, vault, skills, MCP, channels, and contacts are not written.

## Reasons

| Code | When |
|---|---|
| `unchanged` | the stored body and clocks already match this row |
| `duplicate` | an earlier row lost to a later row with the same natural key, or the same destination body |
| `tenant-mismatch` | `tenant_id` is not `--source-tenant` |
| `agent-missing` | `agent_id` is not an agent in `--tenant` |
| `agent-other-tenant` | `agent_id` belongs to an agent in a different tenant |
| `scope-conflict` | two agents disagree on the same user id and file name |
| `invalid` | empty content, bad file name, bad user id, empty agent id, or a clock that does not parse |
| `too-large` | one body over 32KiB, or the owner cap of 16 files or 64KiB |

## Tests

`TestMinhAnFixture` on memory, SQLite, and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set): dry-run calls `PutContextFile` zero times; apply puts each agent file on the mapped agent and each user file on that `user_id`; source clocks are the stored clocks; the GoClaw uuid is not the stored id; a second apply does not write; a file for another source tenant is absent; an agent id that exists only in another tenant is not written there, and a user file for that agent is not written onto the customer; a pre-existing file in that other tenant is unchanged. With `GOSO_CONTEXT_FILES` unset, the pipeline system message does not contain the imported text. With the flag on, `contextfile.Prompt` for that agent and user is a substring of the system message the runner passes to the model. The scripted provider replies from that text. No network model is called. A Minh An memory row and `agents.instructions` are unchanged.

`TestDuplicateLastWinsIsIdempotent` checks the natural key and a later body that keeps one row, the same id, and the source `updated_at`. `TestUserFileConflictDoesNotOverwrite` checks two agents that disagree, and two agents that agree. `TestJSONLUsesTheSameColumns` checks one object per line. `TestOwnerCapAndTooLarge` checks the 17th file, a body over 32KiB, and the 64KiB owner total. `TestInvalidRowsWriteNothing` checks path names, user ids that are not opaque keys, and a bad clock.

## Non-goals

No control-plane change. No `--include-personal`. No personal-document import. No pgvector. No MQ Capital fixture. No GoClaw memory table. No copy of GoClaw or Dewee source. Channel prompts are unchanged. No live connection to the GoClaw database.

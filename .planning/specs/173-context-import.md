# SPEC 173 — GoClaw context-file import

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

This is item 2 of the GoClaw to Goso import (goso#26, goso#28, goso#29). It writes the durable `context_files` store from SPEC 150 (PR #35, `d918e99`). It does not add a table, a column, or a version history. Sample data is Minh An only. This spec does not copy GoClaw.

## Number

| Number | Where it is used |
|---|---|
| 150 | On `main`: durable agent and user context files. This importer writes that table. |
| 152 | On `main`: vault import file bodies. The command shape follows that importer. |
| 173 | This spec. |

## Assumption

Issues #26, #28, and #29 name the GoClaw tables `agent_context_files` and `user_context_files`. They do not list columns. This repo does not either. The JSON below is the input this tool accepts. It is an assumption. The only fields read are the ones `context_files` can store.

```json
{
  "agent_context_files": [
    {"tenant_id": "", "agent_key": "", "name": "", "body": ""}
  ],
  "user_context_files": [
    {"tenant_id": "", "user_key": "", "name": "", "body": ""}
  ]
}
```

`agent_key` is not a `context_files` column. It names an agent that already exists in `--tenant`. The stored owner is that agent's id. `user_key` is stored as the owner. Scope is which array the row is in. Any other JSON field, including an embedding, is ignored. The tool does not create agents.

The checked-in sample is `gateway/internal/contextimport/testdata/minh-an.json`. The agents and users in it are fake Minh An fixtures.

## Behavior

1. `goso-context-import` takes `--export`, `--source-tenant`, `--tenant`, and `--db`. `--tenant` is the goso tenant id and is required. It is not defaulted to `default`. `--source-tenant` is the export `tenant_id` to keep. A row whose `tenant_id` does not match is `tenant-mismatch` and is not written.
2. The default is a dry-run. It reports the same counts and does not call `PutContextFile`. `--apply` writes. `--apply` together with `--dry-run` is an error. `--dry-run=false` without `--apply` still does not write.
3. `--db` is the only database. A Postgres host other than `localhost`, `127.0.0.1`, or `::1` is refused. `GOSO_DATABASE_URL` is not opened. An export larger than 32MiB is refused before decode.
4. An agent file is stored only when `agent_key` matches an agent in `--tenant`. The same key on an agent in another tenant is `agent-other-tenant` and is not written onto that agent. A missing key is `agent-missing`. A user file is stored on `user_key` in `--tenant` only. The same user key in another tenant is a different row and is left as it was.
5. A name must be one path segment of 1–64 characters, with no slash and no `..`. An empty body, a bad name, or a user key that is not an opaque id is `invalid`. A body over 32KiB, or an owner who would pass 16 files or 64KiB, is `too-large`. Nothing is truncated. Earlier rows from the same run stay.
6. Rows that share a destination owner and name collapse to the last valid row. Earlier copies are `duplicate`. A later run of the same export skips every kept row as `unchanged`: no second row, and the stored id, `created_at`, and `updated_at` stay as they were. A different body replaces that same row. There is no version table. A hand edit in goso is replaced when the export body differs, because the body is the instruction text and a provenance marker would be injected into the prompt.
7. The report splits agent rows and user rows by the export `tenant_id`. For each section, `input = inserted + updated + skipped + rejected`. Logs and the report contain names, owner keys, and reason codes. They do not contain file bodies.
8. `GOSO_CONTEXT_FILES` stays default off. The importer writes the table either way. Chat does not read it until the flag is on. `agents.instructions`, memory, vault, skills, MCP, channels, and contacts are not written.

## Reasons

| Code | When |
|---|---|
| `unchanged` | the stored body is already the body this run would write |
| `duplicate` | an earlier valid row lost to a later row with the same owner and name |
| `tenant-mismatch` | `tenant_id` is not `--source-tenant` |
| `agent-missing` | `agent_key` is not an agent in `--tenant` |
| `agent-other-tenant` | `agent_key` belongs to an agent in a different tenant |
| `invalid` | empty body, bad name, bad user key, or empty agent key |
| `too-large` | one body over 32KiB, or the owner cap of 16 files or 64KiB |

## Tests

`TestMinhAnFixture` on memory, SQLite, and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set): dry-run calls `PutContextFile` zero times; apply puts each file on the matching agent or user; a second apply does not write and does not change id or timestamps; a file for another source tenant is absent; an agent key that exists only in another tenant is not written there; a pre-existing file in that other tenant is unchanged; the same user key in the other tenant keeps its own body. With `GOSO_CONTEXT_FILES` unset, the pipeline system message does not contain the imported text. With the flag on, `contextfile.Prompt` for that agent and user is a substring of the system message the runner passes to the model. The scripted provider replies from that text. No network model is called. A Minh An memory row and `agents.instructions` are unchanged.

`TestDuplicateLastWinsIsIdempotent` checks last-wins and a later body change that keeps one row and the same id. `TestOwnerCapAndTooLarge` checks the 17th file, a body over 32KiB, and the 64KiB owner total. `TestInvalidRowsWriteNothing` checks path names and user keys that are not opaque ids.

## Non-goals

No control-plane change. No `--include-personal`. No personal-document import. No pgvector. No MQ Capital fixture. No GoClaw memory table. No copy of GoClaw or Dewee source. Channel prompts are unchanged.

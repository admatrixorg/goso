# SPEC 180 — GoClaw memory_documents import

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

Sample data is Minh An only. This spec does not read a live GoClaw database, does not copy GoClaw code, and does not add a table, a column, or pgvector.

CTO revision 2 (12/11 20:59 UTC+7) replaces the first PR B note. `team_id` and `custom_scope` are empty in the measured export. The skip counter stays. The branch that used to hold those rows back is gone.

## Number

| Number | Where it is used |
|---|---|
| 174 | Separate open PR. Agent importer. This spec does not import that branch. It reads an agent-id file. |
| 179 | Episodic summaries. Not this PR. Customer key rule used here: do not guess `channel_contacts`. |
| 180 | This spec. |
| 152 | Tenant pair reused as a list in the mapping file: GoClaw `tenant_id` → Goso `tenant_id`. |

## Columns

`memory_documents` (Postgres `\d`, 2026-11-12). No column below is assumed.

| Column | Type | Null | Used |
|---|---|---|---|
| `id` | uuid | no | Ignored for the Goso id. The export id is not the vault or memory id. |
| `agent_id` | uuid | no | Looked up in the agent map. |
| `user_id` | varchar(255) | yes | Empty → vault. Non-empty → durable memory. |
| `path` | varchar(500) | no | Vault path and the first line of a durable body. |
| `content` | text | no | Vault body, or the rest of a durable body. Default `''`. |
| `hash` | varchar(64) | no | Compared with an existing vault row. The stored vault hash is sha256 of the bytes written. |
| `created_at` | timestamptz | yes | Durable `created_at` and the archived session time when it parses as RFC3339. |
| `updated_at` | timestamptz | yes | Used only when `created_at` is missing or does not parse. |
| `team_id` | uuid | yes | Non-empty → `team/scope`, not imported. |
| `tenant_id` | uuid | no | Looked up in the tenant map. |
| `custom_scope` | varchar(255) | yes | Non-empty → `team/scope`, not imported. |

`memory_chunks` is not imported. A JSONL object that carries `document_id`, `start_line`, `end_line`, `embedding`, `tsv`, or `text` is a chunk. Its text and embedding are not stored. There is no pgvector write.

Measured paths, not enforced as a quota: top-level `_system/`, `memory/`, and root files. Nested names under those two folders are still classified the same way. Any other folder is `path` and is not imported.

## Behavior

1. Input is a JSONL file, one object per line. The tool does not open a GoClaw DSN. `--db` is the local Goso database (SQLite file or loopback Postgres). A non-local Postgres host is refused. `GOSO_DATABASE_URL` is not read.
2. Empty `user_id` (JSON null, missing, or `""`) and a path in `memory/` or at the root is written with `PutTenantAt` at `memory/goclaw/<goso agent id>/<source path>`. `memory/notes.md` becomes `memory/goclaw/<agent>/memory/notes.md` (the source folder is kept, and a second `memory/` is not inserted). A root file `POLICY.md` becomes `memory/goclaw/<agent>/POLICY.md`. `_system/SOUL.md` is not written, so it does not become `memory/goclaw/<agent>/memory/_system/...`. `..`, a backslash, an absolute path, a `file:` path, and `%2e` are `path`.
3. Non-empty `user_id` on `memory/` or a root file is a `memories` row, `kind=durable`. The body is the cleaned source path, a newline, then `content`. It is not a vault file. Session id `gc-docsess:<sha>`. Memory id `gc-doc:<agent>:<sha>`. The session is archived. `sha` is sha256 hex of the two strings joined by a NUL byte: session = sha(Goso agent id, user_id); memory = sha(user_id, cleaned source path).
4. `customer_key` is the `customers` entry for that `user_id`, or `goclaw:<user_id>` when the entry is absent. It is never `""`. This spec does not read `channel_contacts` and does not guess a match. Only an empty `user_id` stays off a customer key, and that row is the vault row, not a memory.
5. `_system/` is not imported. `--system-names-out` writes the file names (last path segment, empty `user_id` only) to that path with mode 0600. Stdout is counts only and does not include those names, user ids, or channel ids. A non-empty `user_id` under `_system/` is counted as `system` and its name is not written.
6. `team_id` or `custom_scope` set to a non-empty string increments `team/scope` and is not imported. There is no other branch for those columns. JSON null and `""` are empty.
7. `--tenant` is required and names the one source tenant for the run. Other source tenants in the file are `other-tenant` and are not written. A local denylist (`--deny-file` or `GOSO_TENANT_DENYLIST`) is required. A missing, empty, or malformed file stops the process before any work, including `--dry-run`. If `--tenant` or a source tenant in the export is on the list, the run stops before any write. The report prints `denied` as a count and does not print the denied names. This program does not contain a tenant denylist of its own.
8. The agent map points a GoClaw `agent_id` at an existing Goso agent id in that dest tenant. A missing agent is `unmapped`. This spec does not create agents.
9. Vault id is `gc-vdoc:<sha(dest tenant, goso agent id, logical path)>`. The agent slug is one path segment; `..` is rejected. `PutVaultDoc` keeps that id on insert. An empty id still gets a generated id, which is what every other caller does. The same bytes already stored are `hash` and are not written again. A target that already exists and differs is `native-exists` and is not updated.
10. A second run inserts nothing: `will_write` is 0 and `sessions` is 0. Existing `gc-` rows are not updated (`ON CONFLICT (id) DO NOTHING`).
11. `--dry-run` writes nothing. For each source tenant and for `memory_documents` and `memory_chunks`, it reports `read`, `will_write`, and skip counts. `read = will_write +` the reason counters. `expired`, `customer-private`, and `missing-relation-endpoint` stay 0: documents do not expire, a non-empty user is imported, and this spec has no graph edge.
12. `--remove-tenant <goso tenant>` deletes only this importer's ids in that tenant: memories `gc-doc:`, sessions `gc-docsess:` that have no remaining note or message, and vault docs `gc-vdoc:` plus their files. SQLite matches those prefixes with `GLOB` (case-sensitive). Postgres uses `LIKE`. A `gc-ep:` row is not deleted. Another tenant is untouched. A live row is untouched, including a live note on an imported session. That session is kept because the live note still points at it.

Content over 1 MiB, invalid UTF-8, or a NUL byte is not imported (`too-large` or `binary`).

## Mapping file

```json
{
  "tenants": [
    {"source_id": "<goclaw tenant uuid>", "slug": "minh-an", "dest": "minh-an"}
  ],
  "agents": [
    {"source_id": "<goclaw agent uuid>", "source_tenant": "<goclaw tenant uuid>", "dest_id": "<existing goso agent id>"}
  ],
  "customers": [
    {"user_id": "<goclaw user_id>", "customer_key": "zalo-personal:minh"}
  ]
}
```

`tenants` is the SPEC 152 pair, one object per source tenant, plus `slug`. `agents` is the SPEC 174 id map in this shape; the operator writes it, or that importer's output is converted to it. Customer keys are the contacts importer's TSV (`--customer-map`): `channel`, `sender`, `user`, `key`, separated by tabs. Channel names are normalized to the live catalog (`zalo_personal` → `zalo-personal`, `lark` → `feishu`, and the same for the other catalog names) before the key is used. A memory `user_id` matches the user column first, then the sender column. No match becomes `goclaw:<user_id>`, never empty. Unknown JSON fields in the tenant map are rejected. The files contain ids only, not document bodies.

## Command

```text
goso-memdoc-import --export file.jsonl --map map.json --tenant <source id or slug> --deny-file tenants.txt --db path --vault-dir dir [--customer-map contacts.json] [--system-names-out names.txt] [--dry-run]
goso-memdoc-import --remove-tenant minh-an --deny-file tenants.txt --db path --vault-dir dir
```

## Tests

`TestVaultPathShapes` checks the three path shapes and traversal. `TestMinhAnImportAndRemoval` on the memory store, SQLite, and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set): vault vs durable split, `_system/` names only on dry-run, chunk and mq and team/scope not stored, tenant A does not see tenant B, customer X recall does not return customer Y or a `goclaw:` note for a different key, a second run inserts 0, a live episodic summary and a live vault file stay put, removal deletes `gc-` rows and leaves live rows, including a live note on the imported session.

## Non-goals

No control-plane change. No schema change. No import of `memory_chunks`, episodic summaries, or the knowledge graph. No copy of GoClaw. No customer export in the tree.

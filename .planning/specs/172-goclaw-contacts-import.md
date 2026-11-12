# SPEC 172 — Offline contacts import

Status: implemented on this branch. One PR into `main`. Do not merge from this file.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw or Dewee. It does not add a contacts column, a parallel contacts table, or a memory change.

This number is the contacts importer, item 1 of the GoClaw import in issues #26, #28, and #29. It writes the durable tables added in #33.

## Why

Goso can store more than 200 contacts per tenant in `contacts` and `contact_identifiers` when `GOSO_CONTACTS_DURABLE` is on. There is no offline loader for an existing directory. Hung-ai-finance needs that loader before a real import. This PR adds the loader. It does not turn the flag on and it does not import a real directory.

## Input format

The GoClaw `contacts` / `channel_contacts` columns are not listed in this repo or in issues #26, #28, or #29. The file format below is an assumption. Its fields are the goso contact seed. No other field is stored.

```json
{
  "contacts": [
    {
      "tenant_id": "minh-an",
      "channel": "telegram",
      "dest": "ext-minh-an-0000",
      "display": "Minh An 0000",
      "kind": "user",
      "agent_id": "minh-an-sales",
      "seen_count": 1,
      "first_seen": "2026-01-15T08:00:00Z",
      "last_seen": "2026-01-15T09:00:00Z"
    }
  ]
}
```

`dest` is the channel-external id. The natural key is `(tenant_id, channel, dest)`, the primary key of `contact_identifiers`.

An empty `tenant_id` means the row belongs to `--tenant`. A non-empty id that is not `--tenant` is `foreign-tenant` and is not written. Unknown JSON keys are ignored. Phone, email, notes, tokens, and message text are not fields.

`kind` accepts `user`, `direct`, `group`, or empty. `direct` is stored as `user`. Empty uses the directory rule: a dest that starts with `-` is `group`, and any other dest is `user`. Any other kind is `kind` and is not written.

`seen_count` omitted, null, or `0` is stored as 1. A negative count, or a count that is not an integer, is `count`. `first_seen` and `last_seen` are required RFC3339 timestamps.

Channel longer than 64 bytes, dest longer than 128, or display or `agent_id` longer than 256, is invalid. The directory would clip those strings. This importer does not, so two dests cannot collapse into one key. An empty display is stored as `channel:dest`.

## Behavior

1. `goso-contacts-import --export file.json --tenant id --db path` reports counts and writes nothing. `--apply` writes. Passing `--apply` and `--dry-run` together is an error. `--dry-run=false` without `--apply` is an error.
2. The command opens `--db` only. It refuses a non-local Postgres host. It does not follow `GOSO_DATABASE_URL`.
3. A row that is already stored with the same display, kind, agent, count, and timestamps is skipped and is not written again. A difference in those fields updates the contact that owns the key. The contact id stays the same. A second copy of the same key in one file is `duplicate`.
4. The JSON report has `new`, `updated`, `skipped`, `invalid`, and `dry_run`. `reasons` is present only when a row is invalid. Each reason is a code and a count. The report and the error text do not include a display, dest, channel, agent id, or tenant.
5. On `--apply`, the existing store log line is `goso contacts: bulk upsert count=N`. Dry-run does not print that line.
6. Opening `--db` still runs the existing migrations. Dry-run does not insert or update a contact row.
7. Memory, vault, and knowledge-graph tables are not written. `GOSO_CONTACTS_DURABLE` is not read or changed. The gateway serves these rows only when that flag is on.

## Reasons

| Code | When |
|---|---|
| `row` | the object is not a contact object |
| `control` | a field contains a control character |
| `tenant` | `tenant_id` is non-empty and not a tenant id |
| `foreign-tenant` | `tenant_id` is a different tenant |
| `empty-channel` | channel is empty |
| `empty-dest` | dest is empty |
| `channel-length` | channel is over 64 bytes |
| `dest-length` | dest is over 128 bytes |
| `display-length` | display is over 256 bytes |
| `agent-length` | agent id is over 256 bytes |
| `kind` | kind is not user, direct, group, or empty |
| `count` | seen_count is negative or not an integer |
| `time` | a timestamp is missing or not RFC3339 |
| `duplicate` | the same channel and dest already appeared in this file |

## Out of scope

Real tenant directories. Turning `GOSO_CONTACTS_DURABLE` on. MQ Capital. pgvector. Channel credentials. A UI change. Memory, vault, skills, MCP, cron, sessions, and messages.

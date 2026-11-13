# SPEC 172 — Offline contacts import

Status: implemented on this branch. One PR into `main`. Do not merge from this file.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw. It does not add a contacts column or a memory change.

## Source

GoClaw has no `contacts` table. Contacts are `public.channel_contacts`. Columns used by this importer:

`tenant_id`, `channel_type`, `channel_instance`, `sender_id`, `user_id`, `display_name`, `username`, `avatar_url`, `peer_kind`, `metadata`, `merged_id`, `first_seen_at`, `last_seen_at`, `contact_type`, `thread_id`, `thread_type`.

The row `id` is read only to detect `merged_id` pointing at itself. The source unique key is `(tenant_id, channel_type, sender_id, COALESCE(thread_id, ''))`.

## Mapping

goso's key stays `(tenant_id, channel, dest)` on `contact_identifiers`. No source column is added.

| Source | goso |
|---|---|
| `tenant_id` | Filter only. `--source-tenant` is required. The written tenant is `--tenant`. Any other row id is `foreign-tenant`. |
| `channel_type` | `channel`, via a fixed rename (`zalo_personal` → `zalo-personal`, `lark` → `feishu`, and the other catalog names). Unknown names are rejected. |
| `sender_id` | `dest` for the person. This is the id inbound observe uses. |
| `thread_id` | A second `dest` when it differs from `sender_id` (the chat id). |
| `display_name`, else `username` | `display`. Both empty becomes `channel:dest`. |
| `contact_type`, `peer_kind`, `thread_type` | `kind`. `group` when any of the three is `group`. Otherwise `contact_type` must be `user`, `direct`, or empty, and `kind` is `user`. |
| `first_seen_at`, `last_seen_at` | `first_seen`, `last_seen` |
| `merged_id` | Non-empty and not equal to `id`: skip. Not inserted. goso merge rows are not created. Equal to `id`: `merged-self`. |
| `user_id` | Not an `agent_id`. It is a column in the local `--customer-map` file. The key is `<channel>:<sender_id>`, or `goclaw:<user_id>` when the sender is empty. |
| `channel_instance`, `avatar_url`, `metadata` | Not stored. |

`dest` is the raw sender id, plus the raw thread id when that id is different. A later observe of that sender returns the imported row. `dest` over 128 bytes is invalid and is not clipped.

A new row stores `seen_count` 1. An existing contact is `native-exists` and is not changed. Inserted ids are stored in `contact_import_map`. `--customer-map` is required with `--apply`. The file is mode `0600` and the line is `channel`, `sender_id`, `user_id`, `customer_key` separated by tabs. `--rollback` is a dry-run unless `--apply` is also set. It deletes only mapped rows for that tenant. A mapped contact with `seen_count` above 1, `merged` set, or a merge row is active. Active contacts are not deleted unless `--confirm-rollback` is also set. The rollback line is counts only.

`--deny-file` or `GOSO_CONTACTS_DENY_FILE` is required. A missing, empty, or malformed file stops before any work. A listed tenant stops before any write. The report field `denied` is a count and never a name.

The importer lists the destination tenant once and calls `BulkUpsertContacts` once.

## File

JSON:

```json
{
  "channel_contacts": [
    {
      "tenant_id": "minh-an",
      "channel_type": "telegram",
      "sender_id": "ext-minh-an-0000",
      "display_name": "Minh An 0000",
      "contact_type": "user",
      "peer_kind": "user",
      "thread_id": "",
      "first_seen_at": "2026-01-15T08:00:00Z",
      "last_seen_at": "2026-01-15T09:00:00Z"
    }
  ]
}
```

JSONL is one object per line, same keys. A missing `thread_id`, JSON null, and `""` are the same empty thread.

The sample file has 211 rows: 200 direct, 10 threads (one reuses `ext-minh-an-0000` with `thread-minh-an-0000`, one is a group), and 1 merged row. The apply writes 210. The merged row is skipped.

## Command

`--export`, `--tenant`, and `--db` are required. Dry-run is the default. `--apply` writes. `--source-tenant` is optional. The command opens `--db` only and refuses a non-local Postgres host.

## Out of scope

Real directories. Turning `GOSO_CONTACTS_DURABLE` on. Tenants on the local denylist. `--include-personal`. pgvector. Copying merge history. Memory, vault, skills, MCP, cron, channels, sessions, and messages.

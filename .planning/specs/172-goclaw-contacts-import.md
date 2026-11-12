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
| `tenant_id` | Filter only. The written tenant is `--tenant`. `--source-tenant` keeps that source id; every other id is `foreign-tenant`. With no `--source-tenant`, an empty `tenant_id` uses `--tenant` and any other id must equal `--tenant`. |
| `channel_type` | `channel` |
| `sender_id`, `thread_id` | `dest`, see below |
| `display_name`, else `username` | `display`. Both empty becomes `channel:dest`. |
| `contact_type`, `peer_kind`, `thread_type` | `kind`. `group` when any of the three is `group`. Otherwise `contact_type` must be `user`, `direct`, or empty, and `kind` is `user`. |
| `first_seen_at`, `last_seen_at` | `first_seen`, `last_seen` |
| `merged_id` | Non-empty and not equal to `id`: skip. Not inserted. goso merge rows are not created. Equal to `id`: `merged-self`. |
| `user_id`, `channel_instance`, `avatar_url`, `metadata` | Not stored. `user_id` is not `agent_id`. |

```text
dest = "s" + len(sender_id) + ":" + sender_id + "t" + len(thread_id) + ":" + thread_id
```

Lengths are bytes of the trimmed values. An empty thread and a non-empty thread for the same sender are different dests. The length prefix stops a sender that contains `t0:` from colliding with another pair. `dest` over 128 bytes, `sender_id` over 255, or `thread_id` over 100 is invalid and is not clipped.

A new row stores `seen_count` 1. An update keeps the stored count and `agent_id`, because the export has neither. `first_seen` and `last_seen` always come from the file.

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

Real directories. Turning `GOSO_CONTACTS_DURABLE` on. MQ Capital. `--include-personal`. pgvector. Copying merge history. Memory, vault, skills, MCP, cron, channels, sessions, and messages.

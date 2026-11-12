# SPEC 172 — offline contacts import

Offline importer for one tenant into the durable `contacts` and `contact_identifiers` tables from #33. No UI change, so prototype screenshots do not apply and `qa:tabs` was not run. No customer data and no secret values. Env vars are named, not filled in.

The source is `public.channel_contacts`. There is no `contacts` table. goso's tables and key are unchanged.

## Command

| Flag | Meaning |
|---|---|
| `--export` | JSON `{"channel_contacts":[...]}` or JSONL of those objects. Required |
| `--tenant` | goso tenant id written on every kept row. Required |
| `--source-tenant` | Optional `channel_contacts.tenant_id` to keep. Any other id is `foreign-tenant` |
| `--db` | Local SQLite file or loopback Postgres DSN. Required. `GOSO_DATABASE_URL` is ignored |
| `--apply` | Write new and updated rows |
| `--dry-run` | Report counts and write nothing. This is the default |

Do not pass `--apply` and `--dry-run` together. `--dry-run=false` without `--apply` does not write.

```bash
go run ./gateway/cmd/goso-contacts-import \
  --export gateway/internal/contactsimport/testdata/minh-an-contacts.json \
  --tenant minh-an \
  --db /tmp/minh-an-contacts.db
```

Add `--apply` to write. A non-local Postgres host is refused. Opening `--db` still runs the existing migrations. Dry-run does not insert or update a contact row.

`GOSO_CONTACTS_DURABLE` is unchanged and still defaults off. The gateway serves these rows only when that flag is on. This command does not turn it on.

## Mapping

Source unique key: `(tenant_id, channel_type, sender_id, COALESCE(thread_id, ''))`.

goso key: `(tenant_id, channel, dest)`.

```text
channel = channel_type
dest    = "s" + len(sender_id) + ":" + sender_id + "t" + len(thread_id) + ":" + thread_id
```

Lengths are bytes. `ext-minh-an-0000` with an empty thread and the same sender with `thread-minh-an-0000` are two contacts. A sender that contains the characters `t0:` does not collide with another pair, because both lengths are written in front of the values. `dest` over 128 bytes is `dest-length` and is not clipped.

| Source column | Where it goes |
|---|---|
| `channel_type` | `channel` |
| `sender_id`, `thread_id` | `dest` |
| `display_name`, else `username` | `display` |
| `contact_type`, `peer_kind`, `thread_type` | `kind`: `group` if any of them is `group`; otherwise `contact_type` is `user`, `direct`, or empty |
| `first_seen_at`, `last_seen_at` | `first_seen`, `last_seen` |
| `merged_id` | Skip. The row is not inserted. goso does not copy the merge edge. `merged_id` equal to `id` is `merged-self` |
| `user_id` | Not stored. It is not `agent_id` |
| `channel_instance`, `avatar_url`, `metadata` | Not stored |

A new row stores count 1. An update keeps the stored count and `agent_id`. The file's timestamps are written on insert and on update.

One run calls `ListContacts` once and `BulkUpsertContacts` once. A 3300-row file is one batch.

The sample file has 211 rows: 200 direct, 10 threads, 1 merged. The written count is 210.

## Counts

`new + updated + skipped + invalid` equals the number of input rows. Reason codes other than `merged` sum to `invalid`. `merged` is a skipped row.

| Code | When |
|---|---|
| `row` | the value is not a contact object |
| `control` | a kept field contains a control character |
| `tenant` | `tenant_id` is not a tenant id, and `--source-tenant` is unset |
| `foreign-tenant` | the row's tenant is not the one this run keeps |
| `merged` | `merged_id` is set. Counted in `skipped` |
| `merged-self` | `merged_id` equals `id` |
| `empty-channel` | `channel_type` is empty |
| `empty-sender` | `sender_id` is empty |
| `channel-length` | `channel_type` is over 64 bytes |
| `sender-length` | `sender_id` is over 255 bytes |
| `thread-length` | `thread_id` is over 100 bytes |
| `dest-length` | the encoded dest is over 128 bytes |
| `display-length` | the display is over 256 bytes |
| `kind` | `contact_type` is not user, direct, group, or empty |
| `time` | a timestamp is missing or not RFC3339 |
| `duplicate` | the same channel, sender, and thread already appeared in this file |

## Why the earlier test wrote 215 rows

The fixture file had 210 contacts. `TestImportMinhAnSQLite` did not import that file alone. It appended five valid rows that existed only in the test: an empty `tenant_id`, a row whose extra JSON keys were dropped, `seen_count` 0, a dest starting with `-` and an empty kind, and an empty display. 210 + 5 = 215, which is the `bulk upsert count=215` line. The fixture on disk stayed 210. The current fixture file has 211 rows. The apply writes 210 and skips the merged row. Those edge rows are a separate test.

## Dry-run sample

Command above, no `--apply`. Stderr was empty. The SQLite `contacts` count stayed 0.

```json
{
  "new": 210,
  "updated": 0,
  "skipped": 1,
  "invalid": 0,
  "dry_run": true,
  "reasons": [
    {
      "code": "merged",
      "count": 1
    }
  ]
}
```

The skipped row is the sample contact whose `merged_id` is set.

## Tests

Postgres 16.15 local. `GOSO_TEST_POSTGRES_DSN` was set. The value is not recorded. SQLite always runs. Postgres ran.

```bash
go test -race ./gateway/internal/contactsimport ./gateway/cmd/goso-contacts-import -count=1 -timeout 180s
```

```text
ok  	github.com/mqglobal/goso/gateway/internal/contactsimport	12.197s
ok  	github.com/mqglobal/goso/gateway/cmd/goso-contacts-import	2.549s
```

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. `gateway/internal/contactsimport` 1.445s. `gateway/cmd/goso-contacts-import` 0.386s. Logs: [go-test.txt](go-test.txt), [go-test-race.txt](go-test-race.txt).

`TestImportMinhAnSQLite` and `TestImportMinhAnPostgres` write 210 of the 211 fixture rows, leave the merged row out, keep the shared sender and its thread as two contacts, and keep a pre-seeded second tenant unchanged. `TestBatch3300SQLite` and `TestBatch3300Postgres` import 3300 rows with one `ListContacts` and one `BulkUpsertContacts`. A rerun does not call the upsert again.

`OpenPostgres` logs `CREATE EXTENSION vector skipped` when the server has no pgvector. This PR does not install or use pgvector.

## qa:tabs

No control-plane tab, route, or client state changed. `npm run qa:tabs` was not run.

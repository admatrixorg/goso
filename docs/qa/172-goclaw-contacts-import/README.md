# SPEC 172 — offline contacts import

Offline importer for one tenant into the durable `contacts` and `contact_identifiers` tables from #33. No tab changed, so there are no prototype screenshots. `qa:tabs` was run on the code commit and the log is under `docs/qa/172/`. No customer data and no secret values. Env vars are named, not filled in.

The source is `public.channel_contacts`. There is no `contacts` table. goso's tables and key are unchanged.

## Command

| Flag | Meaning |
|---|---|
| `--export` | JSON `{"channel_contacts":[...]}` or JSONL of those objects. Required |
| `--tenant` | goso tenant id written on every kept row. Required |
| `--source-tenant` | Required `channel_contacts.tenant_id` to keep. Any other id is `foreign-tenant` |
| `--deny-file` | Required local denylist, or `GOSO_CONTACTS_DENY_FILE`. One id per line |
| `--customer-map` | Local TSV. Required with `--apply`. Written as mode `0600` |
| `--rollback` | Count mapped rows for `--tenant`. Deletes them only with `--apply` |
| `--confirm-rollback` | Allow delete when a mapped contact was observed again or merged |
| `--db` | Local SQLite file or loopback Postgres DSN. Required. `GOSO_DATABASE_URL` is ignored |
| `--apply` | Write new rows, or delete mapped rows when `--rollback` is set |
| `--dry-run` | Report counts and write nothing. This is the default |

Do not pass `--apply` and `--dry-run` together. `--dry-run=false` without `--apply` does not write.

```bash
go run ./gateway/cmd/goso-contacts-import \
  --export gateway/internal/contactsimport/testdata/minh-an-contacts.json \
  --tenant minh-an \
  --source-tenant minh-an \
  --deny-file /tmp/deny.txt \
  --db /tmp/minh-an-contacts.db
```

Add `--apply` to write. A non-local Postgres host is refused. Opening `--db` still runs the existing migrations. Dry-run does not insert or update a contact row.

`GOSO_CONTACTS_DURABLE` is unchanged and still defaults off. The gateway serves these rows only when that flag is on. This command does not turn it on.

## Mapping

Source unique key: `(tenant_id, channel_type, sender_id, COALESCE(thread_id, ''))`.

goso key: `(tenant_id, channel, dest)`.

```text
channel = renamed goso name (zalo_personal → zalo-personal; unknown names are rejected)
dest    = sender_id, and also thread_id when that id is different
```

`ext-minh-an-0000` and the same sender with `thread-minh-an-0000` are two contacts. The person dest is the raw sender id, which is what a later observe uses. `dest` over 128 bytes is `dest-length` and is not clipped.

| Source column | Where it goes |
|---|---|
| `channel_type` | `channel` after the fixed rename |
| `sender_id` | `dest` of the person |
| `thread_id` | extra `dest` when it differs |
| `display_name`, else `username` | `display` |
| `contact_type`, `peer_kind`, `thread_type` | `kind`: `group` if any of them is `group`; otherwise `contact_type` is `user`, `direct`, or empty |
| `first_seen_at`, `last_seen_at` | `first_seen`, `last_seen` on insert |
| `merged_id` | Skip. The row is not inserted. `merged_id` equal to `id` is `merged-self` |
| `user_id` | Local `--customer-map` only. Key `<channel>:<sender_id>`, or `goclaw:<user_id>` when the sender is empty |
| `channel_instance`, `avatar_url`, `metadata` | Not stored |

An existing contact is `native-exists` and is not changed. New rows are listed in `contact_import_map`. `--customer-map` is required with `--apply` and the file mode is `0600`. The TSV columns are `channel`, `sender_id`, `user_id`, and `customer_key`. `--rollback` without `--apply` prints `dry_run`, `delete`, and `active` and writes nothing. `--apply` deletes only mapped rows for that tenant. `active` counts mapped contacts whose `seen_count` is above 1, whose `merged` flag is set, or that appear in `contact_merges`. Those rows stay unless `--confirm-rollback` is also set. The error text is `rollback needs confirm` and names no contact. Another tenant's rows stay.

`--source-tenant` and a denylist file are required. A missing, empty, or malformed denylist stops before any work. A listed `--tenant` or source tenant stops before any write. `denied` in the report is a count.

One run calls `ListContacts` once and `BulkUpsertContacts` once. A 3300-row file is one batch.

The sample file has 211 rows: 200 direct, 10 threads, 1 merged. The row report's `new` count is 210. Thread rows also insert the chat id, so the stored contact count is higher.

## Counts

`new + updated + skipped + invalid` equals the number of input rows. Reason codes other than `merged` sum to `invalid`. `merged` is a skipped row.

| Code | When |
|---|---|
| `row` | the value is not a contact object |
| `control` | a kept field contains a control character |
| `tenant` | `tenant_id` is not a tenant id, and `--source-tenant` is unset |
| `foreign-tenant` | the row's tenant is not the one this run keeps |
| `merged` | `merged_id` is set. Counted in `skipped` |
| `native-exists` | `(channel, dest)` is already stored. Not updated. Counted in `skipped` |
| `channel` | `channel_type` is not in the rename table |
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
  "denied": 0,
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
ok  	github.com/mqglobal/goso/gateway/internal/contactsimport	11.993s
ok  	github.com/mqglobal/goso/gateway/cmd/goso-contacts-import	2.612s
```

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0 on clean tree `ef51fba81b219123a887df5380b489cf88f16030`. `gateway/internal/contactsimport` 1.277s. `gateway/cmd/goso-contacts-import` 0.378s. The commit that adds these transcripts does not change the importer. Logs: [go-test.txt](go-test.txt), [go-test-race.txt](go-test-race.txt).

`TestImportMinhAnSQLite` and `TestImportMinhAnPostgres` write 210 of the 211 fixture rows, leave the merged row out, keep the shared sender and its thread as two contacts, and keep a pre-seeded second tenant unchanged. `TestBatch3300SQLite` and `TestBatch3300Postgres` import 3300 rows with one `ListContacts` and one `BulkUpsertContacts`. A rerun does not call the upsert again.

`OpenPostgres` logs `CREATE EXTENSION vector skipped` when the server has no pgvector. This PR does not install or use pgvector.

## qa:tabs

No control-plane tab, route, or client state changed. The all-tabs check still ran against a gateway built from `2729e4babaa875352b251cc0689073a6249d900f`.

Chrome binary: `/usr/local/bin/google-chrome`. `QA_SKIP_TABS` and `QA_SHOTS` were unset.

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway: `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from that commit. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`. Seed: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Log: [../172/qa-tabs-2729e4babaa875352b251cc0689073a6249d900f.log](../172/qa-tabs-2729e4babaa875352b251cc0689073a6249d900f.log).

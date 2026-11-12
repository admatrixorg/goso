# SPEC 172 — offline contacts import

Offline importer for one tenant into the durable `contacts` and `contact_identifiers` tables from #33. No UI change, so prototype screenshots do not apply and `qa:tabs` was not run. No customer data and no secret values. Env vars are named, not filled in.

The GoClaw `contacts` / `channel_contacts` columns are not listed in this repo or in issues #26, #28, or #29. The JSON shape below is an assumption. It is goso's contact seed. Phone, email, notes, tokens, and message text are not fields. Unknown keys are dropped and are not stored.

## Command

| Flag | Meaning |
|---|---|
| `--export` | JSON file with a `contacts` array. Required |
| `--tenant` | goso tenant id to write. Required. The only tenant this run can write |
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

## Input

Natural key: `(tenant_id, channel, dest)`.

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

An empty `tenant_id` belongs to `--tenant`. Any other tenant id is `foreign-tenant` and is not written. `kind` accepts `user`, `direct`, `group`, or empty. `direct` is stored as `user`. Empty uses the directory rule (a dest that starts with `-` is `group`). `seen_count` omitted, null, or `0` is stored as 1. Both timestamps are required RFC3339. Channel over 64 bytes, dest over 128, or display or agent id over 256 is invalid. An empty display is stored as `channel:dest`.

The checked-in sample is the generator `MinhAnFile` (210 contacts: 200 telegram users, 10 zalo-oa groups). Displays are `Minh An NNNN`. Dests are `ext-minh-an-NNNN` or `grp-minh-an-NNNN`. There are no phone numbers.

`new + updated + skipped + invalid` equals the number of input rows. `reasons` sums to `invalid`.

| Code | When |
|---|---|
| `row` | the JSON value is not a contact object |
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

A matching stored row is skipped. A difference in display, kind, agent, count, or timestamps updates that contact and keeps its id. On `--apply`, stderr may also show the existing store line `goso contacts: bulk upsert count=N`. That line is a count. Dry-run does not print it.

## Dry-run sample

Command above, no `--apply`. Stderr was empty. The new SQLite file had `contacts` count 0.

```json
{
  "new": 210,
  "updated": 0,
  "skipped": 0,
  "invalid": 0,
  "dry_run": true
}
```

`--apply` on that file:

```json
{
  "new": 210,
  "updated": 0,
  "skipped": 0,
  "invalid": 0,
  "dry_run": false
}
```

The same `--apply` again, contact count still 210:

```json
{
  "new": 0,
  "updated": 0,
  "skipped": 210,
  "invalid": 0,
  "dry_run": false
}
```

A dry-run of one kept row, one row for another tenant, and one row with an empty dest:

```json
{
  "new": 1,
  "updated": 0,
  "skipped": 0,
  "invalid": 2,
  "dry_run": true,
  "reasons": [
    {
      "code": "empty-dest",
      "count": 1
    },
    {
      "code": "foreign-tenant",
      "count": 1
    }
  ]
}
```

## Tests

Postgres 16.15 local. `GOSO_TEST_POSTGRES_DSN` was set. The value is not recorded. SQLite always runs. Postgres ran.

`git status` was dirty with these QA files when the first full run finished. The Go sources under test did not change after that run. Exit 0 both times below.

```bash
go test -race ./gateway/internal/contactsimport ./gateway/cmd/goso-contacts-import -count=1 -timeout 180s
```

```text
ok  	github.com/mqglobal/goso/gateway/internal/contactsimport	2.517s
ok  	github.com/mqglobal/goso/gateway/cmd/goso-contacts-import	2.566s
```

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. Every package with tests passed, including `gateway/internal/contactsimport`, `gateway/cmd/goso-contacts-import`, `gateway/internal/store`, `gateway/internal/vaultimport`, and `gateway/memorybench`. Log: [go-test.txt](go-test.txt). Race log: [go-test-race.txt](go-test-race.txt).

`TestImportMinhAnSQLite` and `TestImportMinhAnPostgres` import 215 rows (the 210-contact fixture plus five valid extras), dry-run leaves the count at 0, a second apply keeps that count and the same ids, and a tenant seeded beforehand is unchanged. `TestCLIPostgresApply` does the same 210-row file through the command on Postgres. `OpenPostgres` logs `CREATE EXTENSION vector skipped` when the server has no pgvector. This PR does not install or use pgvector.

## qa:tabs

No control-plane tab, route, or client state changed. `npm run qa:tabs` was not run.

## Out of scope

Real directories. Turning the durable-contacts flag on. MQ Capital. Memory changes. Channel credentials.

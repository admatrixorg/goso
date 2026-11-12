# SPEC 177 — cron and channel import

Offline import of GoClaw `cron_jobs` and `channel_instances` into the existing goso cron jobs and `channel_config`. Sample data is the Minh An fixture only. No live GoClaw database, no credentials, no pgvector, no MQ Capital rows.

## Command

```bash
go run ./gateway/cmd/goso-cron-channel-import \
  --export gateway/internal/scheduleimport/testdata/minh-an.json \
  --source-tenant minh-an \
  --tenant minh-an \
  --db /tmp/minh-an-schedule.db
```

That prints per-tenant counts and writes nothing. Add `--apply` to insert. `--apply` with `--dry-run` exits 2. A non-local Postgres host is refused. `GOSO_DATABASE_URL` is not opened.

The destination tenant needs a session before cron rows can be stored. This command does not create sessions or agents.

## Checks

- Dry-run leaves the cron and channel tables unchanged and still lists `needs_credential`.
- `--apply` stores cron jobs and the channel row disabled. A later tick does not run those jobs.
- A second `--apply` skips the same source ids and does not insert a duplicate.
- A pre-existing cron job and the catalog row `telegram` are not updated.
- Rows whose `tenant_id` is `other-co` are not written when `--source-tenant` is `minh-an`.
- The fixture secret string is absent from the report, the log, and the SQLite file.

`go build` of `goso-gateway` and `goso-cron-channel-import` exited 0.

`go test -count=1 -timeout 25m ./...` exited 0.

`go test -race -count=1` on `gateway/internal/scheduleimport`, `gateway/cmd/goso-cron-channel-import`, and `gateway/internal/store` exited 0. A race of the whole module also passed those packages. `gateway/internal/replycheck` failed `TestCheckClefFailureFailsClosed` with a data race inside that existing test. This change does not touch that package.

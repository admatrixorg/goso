# SPEC 177 — cron and channel import

Offline import of channel instances into the existing `channel_config` table. Cron rows are reported `cron-no-tenant` and are not written. Sample data is the Minh An fixture only. No live source database and no credentials.

## Command

```bash
go run ./gateway/cmd/goso-cron-channel-import \
  --export gateway/internal/scheduleimport/testdata/minh-an.json \
  --source-tenant minh-an \
  --tenant minh-an \
  --deny-file /tmp/deny.txt \
  --db /tmp/minh-an-schedule.db
```

`--deny-file` is required (or `GOSO_IMPORT_DENY_FILE`). A missing, empty, or malformed file exits non-zero before the database is opened. The dry-run opens the database read-only and does not migrate. Add `--apply` to insert channel rows. `--rollback` without `--apply` only counts channel mappings for `--tenant`. `--rollback --apply` deletes a mapped channel only while it is still disabled and has no stored secret. An enabled row, or one with a secret, is kept. A native catalog row and another tenant's rows stay.

## Checks

- Dry-run leaves the cron and channel tables unchanged and still lists `needs_credential`.
- `--apply` stores the channel row disabled and does not write a cron job.
- A second `--apply` skips the same source ids and does not insert a duplicate.
- A pre-existing cron job and the catalog row `telegram` are not updated.
- Rows whose `tenant_id` is `other-co` are not written when `--source-tenant` is `minh-an`.
- The fixture secret string is absent from the report, the log, and the SQLite file.

Code under the browser check: `ecd6bb756a013ec3caf3d9d636ac9a2baa19a58d`. The log is [qa-tabs-ecd6bb756a013ec3caf3d9d636ac9a2baa19a58d.log](../177/qa-tabs-ecd6bb756a013ec3caf3d9d636ac9a2baa19a58d.log). That commit is the rollback-safety fix merged with `origin/main`. Chrome binary `/usr/local/bin/google-chrome`. `QA_SKIP_TABS` was not set. `QA_SHOTS` was not set. Exit 0. Last lines: `flow chat` then `qa:tabs ok`. `go test -count=1 ./...` and `go test -race` on `gateway/internal/scheduleimport` and `gateway/internal/store` exited 0 on that commit.

The rollback-safety commit before that merge, `b067cc7a1d6cc08e42e160bd162b64346e326867`, is [qa-tabs-b067cc7a1d6cc08e42e160bd162b64346e326867.log](../177/qa-tabs-b067cc7a1d6cc08e42e160bd162b64346e326867.log).

An earlier check after merging `origin/main`, `f937fdaa412307cb25acf779e7460ae9a3396739`, is [qa-tabs-f937fdaa412307cb25acf779e7460ae9a3396739.log](../177/qa-tabs-f937fdaa412307cb25acf779e7460ae9a3396739.log).

An earlier check of `151a150f2009331f02738943a89013dcf1476f43` is [qa-tabs-151a150f2009331f02738943a89013dcf1476f43.log](../177/qa-tabs-151a150f2009331f02738943a89013dcf1476f43.log).

An earlier check of `4bfc382ba3dfb4d31a45a11c279d2a0a15043c21` is [qa-tabs-4bfc382ba3dfb4d31a45a11c279d2a0a15043c21.log](../177/qa-tabs-4bfc382ba3dfb4d31a45a11c279d2a0a15043c21.log).

`go build` of `goso-gateway` and `goso-cron-channel-import` exited 0.

`go test -count=1 -timeout 25m ./...` exited 0.

`go test -race -count=1` on `gateway/internal/scheduleimport`, `gateway/cmd/goso-cron-channel-import`, and `gateway/internal/store` exited 0. A race of the whole module also passed those packages. `gateway/internal/replycheck` failed `TestCheckClefFailureFailsClosed` with a data race inside that existing test. This change does not touch that package.

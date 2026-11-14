# SPEC 177 — Import channel instances

Offline importer from a GoClaw export file into goso's existing channel config. No live database connection. Sample data is Minh An only.

The command is `goso-cron-channel-import`. `--source-tenant` and `--tenant` are required. The default prints per-tenant counts and writes nothing. `--apply` writes. `--apply` together with `--dry-run` is refused.

A local denylist is required (`--deny-file` or `GOSO_IMPORT_DENY_FILE`), one tenant id per line. A missing, empty, or malformed file stops the process before it opens the database, including a dry-run. If `--tenant` or `--source-tenant` is on the list, the run stops before any write. The report field `denied_tenants` is a count. Tenant ids from that list are not printed.

## Channels

Every imported channel config has `enabled` false. The name is `<tenant>/<catalog>/<name>` so the seven live catalog rows are not overwritten and this tool does not start them.

`credentials` is not stored. The report lists each kept instance with `needs_credential: true`. Phone numbers and runs of eight or more digits are replaced with a short hash in the report and the log.

A second `--apply` of the same source id does not insert again. If the stored row differs, the reason is `native-exists` and the row is not updated.

## Cron

`cron_jobs` rows are `not_imported` with reason `cron-no-tenant`. They are not written. This tool does not pick a session for them. Per-tenant cron is a later spec.

## Rollback

`--rollback` counts `channel_instance` mappings for the exact `--tenant`. Without `--apply` it writes nothing. With `--apply` it deletes a mapped `channel_config` only while that row is still disabled and has no channel secret, then deletes that mapping. An enabled row or a row with a secret is kept. A native catalog row, other kinds, and other tenants stay.

## Dry-run

A dry-run opens SQLite or loopback Postgres read-only and does not run migrations.

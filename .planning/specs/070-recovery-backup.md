# SPEC 070 — Backup / restore drill

> LOCKED after 069. Clean-room. Do not kill `:8082` `:8091`. Do not touch live demo DB except via copy.

Closes **CTO-10**.

## GoClaw (cite)

- Admin backup/restore HTTP: `POST /v1/system/backup`, download token, `POST /v1/system/restore` with `dry_run` (`docs/18-http-api.md` ~1403–1433).
- Ops: scheduled dump + retain N (`docs/project-changelog.md` backup timer mention — behavior = scheduled consistent dump, not the vendor).

## goso today

- `compose.prod.yml` sidecar copies a **live** SQLite file (unsafe). No `PRAGMA integrity_check`, no restore drill, no admin API.

## goso plan

1. Consistent snapshot: `VACUUM INTO` (or SQLite backup API) to a timestamped file under `GOSO_BACKUP_DIR` (default `./var/backups`). Never `cp` the live db for the supported path.
2. `POST /api/system/backup` (admin) creates snapshot, returns `{file, bytes, integrity: "ok"}`. Run `PRAGMA integrity_check` on the snapshot.
3. `POST /api/system/restore` `{file}` **or** CLI `goso-gateway restore --file` for tests: restore into a **temp** db, integrity_check, optional `--apply` (tests use temp; production apply documented as stop-gateway).
4. Compose backup sidecar uses the same VACUUM INTO path. Document RPO/RTO in `docs/RUNBOOK.md` (no fake vendor SLAs).
5. TLS/alert: notes only + DI-10/18 remain parked. Do not invent Grafana.

## UI

Settings or a small Backup card: button “Tạo bản sao”, list files, integrity badge. i18n. No auto-restore from browser without confirm.

## Tests

- Create sqlite with a row → backup → integrity ok → restore to temp → row present.
- Corrupt copy rejected.
- Live file not exclusively locked/deleted.

QC: typecheck if UI, go test, build, agpl 0, `docs/qa/070-recovery-backup.md`. Commit `admatrixmdp/spec070-recovery-backup`. Do not merge.

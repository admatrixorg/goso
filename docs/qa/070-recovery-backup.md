# QA — SPEC 070 Recovery / backup

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Live demo DB `/tmp/goso-044-demo/goso.db` was not overwritten. Closes **CTO-10**. Do not merge. Do not start SPEC 071.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Admin HTTP backup/restore (`POST /v1/system/backup`, restore `dry_run`) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/18-http-api.md` (~1403–1433) |
| Scheduled dump + retain N | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/project-changelog.md` (backup timer — scheduled consistent dump, retain N; not the vendor) |

goso mapping (self-written): `VACUUM INTO` under `GOSO_BACKUP_DIR` (default `./var/backups`). `POST /api/system/backup` (admin, `/v1` alias) returns `{file, bytes, integrity:"ok"}` after `PRAGMA integrity_check` on the snapshot. Restore drill is a temp db (HTTP `POST /api/system/restore` and `goso-gateway restore --file`). Live apply is CLI `--apply` after stop-gateway. Compose prod sidecar uses `VACUUM INTO` + integrity_check + retain N, not `cp` of the live file. Settings backup card: create, list, integrity badge, confirm before restore drill. i18n vi+en. TLS/alert notes only; DI-10/18 stay parked. No Grafana.

## What changed

- `gateway/internal/backup`: `VACUUM INTO` snapshot, `PRAGMA integrity_check`, list, path jail, temp restore, CLI apply.
- HTTP `GET/POST /api/system/backup`, `POST /api/system/restore` (no live apply from HTTP).
- CLI `goso-gateway restore --file [--apply] [--dest]`.
- `compose.prod.yml` sidecar: calls gateway `POST /api/system/backup` (`VACUUM INTO` + integrity_check) + `BACKUP_RETAIN` (default 14). Gateway mounts `backup:/backup` as `GOSO_BACKUP_DIR`. Unique snapshot names; no `cp`.
- Control-plane Settings → Sao lưu / Backup card.
- `docs/RUNBOOK.md` RPO/RTO, TLS/alert notes. DI-10/18 parked.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 071.

## Proof

- Create sqlite with a row → backup → integrity ok → restore to temp → row present (`TestSnapshotRestoreRoundTrip`).
- Corrupt snapshot rejected (`TestCorruptSnapshotRejected`, HTTP 400).
- Live file remains and stays writable after snapshot (not exclusively locked/deleted).
- Compose sidecar has no `cp` of `/data/goso.db`.

## Non-goals

SPEC 071 tenant-lite. Grafana. Implementing DI-10/18. Binding/killing demo ports. Merge. Copying goclaw Go. Secrets in docs. `cp` of the live db as a supported path.

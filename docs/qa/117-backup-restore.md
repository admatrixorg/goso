# QA — SPEC 117 Backup & Restore

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Backup & Restore: system/tenant backups, preflight, archive validate/restore, optional S3 write-only + `configured` GET; destructive restore confirm; archives exclude credentials | `docs/qa/090-goclaw-sidebar-ux.md` Backup & Restore |

goso mapping (self-written): Settings backup tab in [SettingsPage](../../control-plane/src/pages/SettingsPage.tsx) renders [BackupPanel](../../control-plane/src/pages/BackupPanel.tsx). Preflight is `GET /api/system/backup/preflight` (`/v1` alias) with engine, `can_backup`, blocking database/tool checks (`sqlite_file`, `pg_dump` when Postgres). Create remains `POST /api/system/backup` `{scope,tenant,destination}` — system or tenant SQLite `VACUUM INTO`, then sanitize (secrets table emptied, connector/webhook credential columns cleared). Tenant copies also drop unscoped connectors/channels. List/download/validate/plan: `GET /api/system/backup`, `GET /api/system/backup/download?file=` (sanitized stream), `POST /api/system/backup/validate`, `POST /api/system/restore/plan`. Restore verify is `POST /api/system/restore` `{file}`; `apply=true` requires `confirm` matching the basename and still returns 400 CLI-only. S3 is `GET/PUT /api/system/backup/s3`, `POST .../s3/test`, `POST .../s3/clear` — GET returns `configured` / `access_key_set` / endpoint metadata, never access_key or secret. Env-owned S3 credentials (`GOSO_BACKUP_S3_*`) refuse PUT secrets (409). Issued `write` keys cannot mutate backup/S3 (admin only). Mutations append SPEC 110 audit rows (`entity=backup` / `backup_s3`) without tokens. View-token GET backup 403 (admin-only). CLI `--apply` of a 117 snapshot does not restore credentials — operators re-enter them.

Out of scope: TTS (118). Copying GoClaw chrome. Live vendor tokens. Import/Export (116) portable archives. Binding/killing demo ports. Merge.

## What changed

- Settings backup sub-tabs: system / tenant / restore / S3. Preflight, progress, loading / empty / error.
- System and tenant snapshots; archive validation; restore plan + type-to-confirm; failure recovery via `.pre-restore` + temp cleanup. Optional S3 write-only + `configured` on GET. Archives never include credentials.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/117-backup-restore.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/backup ./gateway/internal/auth ./gateway/internal/httpapi ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicFile` / `asPublicList` dropping token rows, `asPublicS3` refusing `access_key`/`secret`, confirm match helpers.
- `go test` backup: sanitize deletes `secrets` and clears `credential_ref` on the copy (live file unchanged); tenant prune keeps only the requested tenant; postgres without `pg_dump` blocks preflight; S3 GET/public omits secrets; env-owned PUT refused; restore plan + confirm. httpapi: preflight `/v1` alias; S3 PUT never echoes secret; download; validate; plan `confirm_required`; apply without confirm 400; apply with confirm still CLI-only. auth/serve: view GET/POST `/api/system/backup` 403.
- Page copy: empty states per tab. Type-to-confirm before verify restore. Preflight can disable Start. S3 inputs empty on load. Distinct from Import/Export (116).
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

TTS (118). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPEC 116 portable archives. Live HTTP apply of the running SQLite file (still CLI after stop-gateway).

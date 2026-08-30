# QA — SPEC 116 Import & Export

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Import & Export: Teams / Agents / Skills & MCP / Export / Import tabs; select scope, export archive, upload/validate/import; beta labeling and validation/error states. Archives must exclude credentials | `docs/qa/090-goclaw-sidebar-ux.md` Import & Export |

goso mapping (self-written): live tab `impexp` in [App.tsx](../../control-plane/src/App.tsx) renders [ImportExportPage](../../control-plane/src/pages/ImportExportPage.tsx). Catalog is `GET /api/import-export` (`/v1/import-export` alias) returning `{teams,agents,skills,mcp,skills_configured,generated_at}` with MCP `token_set` / `env_owned` booleans only. Agents/teams are tenant-scoped (`X-Goso-Tenant` / default). Schema `goso.portable/v1` is required as sent — missing or unknown version is rejected before strip. Export is `POST /api/import-export/export` `{team_ids,agent_ids,skill_names,mcp_names}` returning a job plus `goso.portable/v1` archive. Preview is `POST /api/import-export/preview` `{archive}`. Import is `POST /api/import-export/import` `{archive,conflict,dry_run}` with conflict `skip|overwrite|rename`. Rollback is `POST /api/import-export/{id}/rollback`. GET job/archive never includes `token` / `secret` / credential_ref `secret:` values. MCP connectors that had credentials are listed in `credentials_needed` so operators re-enter them on Functions. Mutations append SPEC 110 audit rows (`entity=portable`, actions export/import/rollback) without tokens. View-token GET 200 / POST 403. Issued `read` GET 200 / POST 403; `write` can preview/export/import.

Out of scope: Backup (117). TTS (118). Copying GoClaw chrome. Live vendor tokens. SPECs 117–118.

## What changed

- Live nav tab + page. Staged Teams / Agents / Skills & MCP / Export / Import. Loading / empty / error / progress.
- Manifest preview, schema/version validation, conflict strategy, dry run, rollback/reporting. Secrets excluded by default; re-enter credentials after import. GET/archive never returns tokens.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/116-import-export.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/impexp ./gateway/internal/auth ./gateway/internal/httpapi ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicCatalog` dropping `token` rows, `asPublicArchive` refusing `include_secrets`, `asPublicJob` dropping leaks, selection helpers.
- `go test` impexp: strip drops `sk-` / `token` keys; catalog/export omit `secret:` refs; dry run writes nothing; skip/overwrite/rename; boxed secret never appears in GET/archive JSON; rollback removes created agents/teams. httpapi: GET catalog/job omit secrets; `/v1/import-export` alias; smuggled token field does not round-trip; view-token GET 200 / POST 403; issued `read` GET 200 / POST 403, `write` can preview. auth/serve: view GET `/api/import-export` 200, POST 403.
- Page copy: empty states per tab. Confirm checkbox before import. Dry run vs import. Manifest preview. Progress/report. Distinct from Settings backup.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Backup (117). TTS (118). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 117-118. Settings SQLite VACUUM backup (070/117).

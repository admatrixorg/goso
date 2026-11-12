# SPEC 170 — vault upload

Evidence for `POST /api/vault/upload`. No control-plane tab change, so prototype screenshots do not apply. No customer data and no secret values. Env vars are named, not filled in. Fixtures are synthetic Minh An files.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `GOSO_VAULT_UPLOAD` | off | `POST /api/vault/upload` and `GET /api/vault/ingest/{id}` return 404. `LimitAPI` still cuts that POST at 1 MiB |
| `GOSO_VAULT_INGEST_MAX_BYTES` | 20971520 | Per-file cap after auth and tenant resolution, only on the upload route |
| `GOSO_VAULT_INGEST_QUOTA_BYTES` | unset | Per-tenant disk quota. Missing, zero, or negative fails closed (`quota_required`) and writes nothing |

Other `/api` and `/v1` routes stay at 1 MiB whether or not the upload flag is on. There is no `/v1/vault/upload` alias.

## Forget (SPEC 164)

This PR does not delete an upload when a person is forgotten. SPEC 164 still has to delete `raw/`, the ingest job, `sources/`, and derived wiki pages. Turning the flag off does not drop `vault_ingest_jobs` and does not remove files already stored.

## Tests

Postgres for the local `go test` was **16.15** (Ubuntu package `16.15-0ubuntu0.24.04.1`). The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here. This Postgres has no pgvector extension; the store logs that `CREATE EXTENSION vector` was skipped and continues. SQLite cases run in the same command.

From the repo root:

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. Every package reported `ok` or `[no test files]`.

[go-test.txt](go-test.txt)

Verbose cases for the acceptance criteria, with memory, SQLite, and Postgres where the store is involved:

[go-test-cases.txt](go-test-cases.txt)

| Criterion | Test |
|---|---|
| 1. Only `POST /api/vault/upload`, no `/v1` alias | `TestCriterion1_UploadRouteHasNoV1Alias` |
| 2. `LimitAPI` exempts that POST only while the flag is on; flag off still cuts a 2 MiB body at 1 MiB and writes no `raw/`; flag on accepts 20 MiB; other `/api` routes stay at 1 MiB | `TestLimitAPI_VaultUploadExemption`, `TestVaultUpload_FlagOffTwoMiBDoesNotWrite`, `TestVaultUpload_TwentyMiBAndOtherRoutesStayCapped` |
| 3. Admin/write and tenant before the body, including dev-mode with no token. A 20 MiB unauthenticated request is not read | `TestVaultUpload_AuthTenantBeforeBody`, `TestVaultUpload_UnauthenticatedBodyNotRead`, `TestVaultUpload_WriteKeyCannotSwitchTenant` |
| 4. `PutTenantAt` writes the file and the `vault_docs` row together and refuses `raw/`, escape, and symlinks | `TestPutTenantAt_WritesTogetherAndRefusesRaw` |
| 5. `raw/` is out of sync, inventory, and the SPEC 152 importer. Sync deletes no `sources/` or `wiki/` row and creates none from `raw/x.md` | `TestSyncInventory_RawDoesNotTouchSourcesOrWiki`, `TestImportSkipsRaw` |
| 6. Per-tenant disk quota fails closed when missing or exceeded | `TestVaultUpload_QuotaRequiredFailClosed`, quota case inside `TestVaultUpload_MinhAnFilesDuplicateAndQuota` |
| 7. New `vault_ingest_jobs` table only, SQLite and Postgres, existing `vault_docs` columns unchanged | `TestIngestJobTableAdditive` |

`TestVaultUpload_MinhAnFilesDuplicateAndQuota` also covers magic bytes (pdf, docx, xlsx, pptx, png, jpeg, gif, webp, txt, md), a fake extension, `.doc` and `.exe`, same-tenant duplicate, cross-tenant isolation, streaming in chunks no larger than 32 KiB, and the audit row (size, extension, status, no file body). `TestVaultUpload_FileCap413` covers a file over the per-file cap. `TestVaultUpload_FlagOffLeavesPutDocs` covers `PUT /api/vault/docs` with the flag off.

`TestSyncInventory_RawDoesNotTouchSourcesOrWiki` and `TestIngestJobTableAdditive` passed on memory, SQLite, and Postgres.

## qa:tabs

[qa-tabs.log](qa-tabs.log)

Chrome: **Google Chrome 156.0.8078.47**. Binary: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set. `QA_SHOTS` was not set. No vault tab markup changed, so there are no prototype screenshots.

From `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from this tree. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_VAULT_UPLOAD`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

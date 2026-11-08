# SPEC 140 — Vault scoped storage and sync

Status: implemented on `cursor/vault-scoped-storage-sync-e110`.
Plan item: owner roadmap **01 · vault-scoped-storage-sync** (not a SPEC id). Official numbers **134–139** are already named in `.planning/specs/goso-2026-10-31-queue.md`. **140** is the next unused number.
Backend-only. No UI change. Screenshots N/A.

Sample data is Minh An only (sales / pháp chế). This spec does not read customer data and does not copy GoClaw.

The phrase for this capability in new text is **truy vấn kiến thức và dữ liệu xuyên phòng ban**.

## Why

Vault sync walked one shared root and reconciled the whole registry. `FindVaultDocByPath` / `FindVaultDocByTitle` and the SQLite `UPDATE` were not tenant-scoped. Postgres `vault_docs.path` was `UNIQUE` for the whole table. A source-level regression showed sync deleting another tenant's row, and a second put of the same logical path collapsing both tenants onto one row. That is not a confirmed production exploit.

## Goal

Knowledge for one tenant is not readable, writable, or sync-deleted by another tenant, including when the title and logical path are the same. Sync reconciles only that tenant's permitted files. A dry-run inventory reports mapping and reason and does not mutate.

## Behavior

1. Logical `path` is unique per `tenant_id`, on memory, SQLite, and Postgres. New files are stored under `GOSO_VAULT_DIR/t/<tenant>/`, a directory the server assigns from the resolved tenant. The logical path is not an agent, project, or customer grant.
2. `put`, `get`, `search`, links, backlinks, graph, health, and search snippets take the resolved tenant before ranking, `LIMIT`, or pagination. Search does not read a global top 50 and then drop other tenants.
3. `POST /api/vault/sync` calls `SyncTenant` for `tenant.Resolve` only. Query `tenant`, body `tenant_id`, `path`, `agent_id`, `project_id`, and `customer_key` do not select a tenant. A body `tenant_id` that disagrees with the resolved tenant is 403. `..` and absolute paths are 400.
4. Flag **vault_scoped_sync** is `GOSO_VAULT_SCOPED_SYNC=1`. Default off. Off returns `vault sync disabled` and does not run the old global scan. `Sync()` with no tenant always returns that error.
5. Deletes run only after a finished, non-empty scan, and only for rows in that tenant whose owner was recorded by a scoped write and whose namespace file is gone. An empty, failed, or partial scan deletes nothing. A database error is returned; it is not a success. Unclear-owner rows are not deleted.
6. `GET` and `POST /api/vault/inventory` are a dry-run for the resolved tenant. `apply=1` is ignored. Entries are `mapped` (`registry_explicit`) or `quarantine` (`unclear_owner`, `unregistered_file`). A path whose first segment is `agent(s)`, `project(s)`, `customer(s)`, or `area(s)` adds `path_not_authority`. The report does not include another tenant's id, title, or body. Replaying it does not insert rows.

## Permission

This PR does not add staff RBAC. The existing contract stays:

- Admin bearer is the vault caller. `X-Goso-Tenant` is honored only when `GOSO_MULTI_TENANT=1` and the admin token is present. The header is not a staff identity.
- View token: vault routes are not on the view allowlist, so vault calls are 403.
- Issued key `read` cannot PUT or POST vault. Issued key `write` may PUT and POST, and the existing matrix still does not grant that key GET. Scope stays the resolved tenant (the header does not move a write key).

## Acceptance

- `TestSync_TenantADoesNotDeleteB` and `TestSync_TenantADoesNotDeleteB_SQLiteAndPostgres`: syncing sales leaves the legal row.
- `TestSync_TwoTenantsSameTitlePath` and `TestPutVaultDoc_SameLogicalPathTwoTenants` (memory, SQLite, Postgres): same title and path cannot read, write, or collapse.
- `TestSearchVault_TenantBeforeLimit`: 55 other-tenant hits do not hide the in-tenant doc. Limit 50 applies after the tenant predicate.
- `TestVaultLinks_DoNotCrossTenants`: resolver and backlinks stay inside the tenant. Graph and health counts do too.
- `TestVaultAPI_ForgedScopeAndSameTenantPermission`: forged tenant, traversal, view token, read key, and a write-key header do not cross tenants.
- `TestSync_EmptyErrorPartialScanDoesNotMassDelete`, `TestSync_DBFailureIsNotSuccess`, `TestSync_FlagOffDoesNotGlobalSync`.
- `TestVault_PathEscapeSymlinkAbsolute`, `TestNormalizeVaultPath_RejectsEscape`.
- `TestInventory_DryRunDoesNotMutate`: replay does not duplicate or rewrite bytes.
- `TestSQLiteVault_UpgradeDropsGlobalPathUnique`, `TestPostgresVault_UpgradeDropsGlobalPathUnique`: old global `UNIQUE(path)` upgrades without assigning `owner_explicit` or rewriting the legacy body.

## Non-goals

Converters, PARA, taxonomy, AI, moving legacy files, assigning owners, deleting unclear-owner docs, applying quarantine, per-document ACL, and a staff directory. No UI.

## Migration

Additive `owner_explicit INTEGER NOT NULL DEFAULT 0`. Existing rows stay 0. SQLite rebuilds the table only when `path` is still globally unique; rows are copied, files stay put. Postgres drops `vault_docs_path_key` if present and adds a unique `(tenant_id, path)` index. Neither path sets a tenant on an unclear row or deletes one.

## Rollback

Turn `GOSO_VAULT_SCOPED_SYNC` off. New sync calls fail closed. Scoped reads and writes stay in place. Documents, including unclear-owner rows and files at the old root, are left where they are. There is no return to a global delete.

## Gaps

- No separate platform principal, so the dry-run cannot list shared-root filenames without showing them to a tenant. Those files are left untouched and are not auto-imported.
- Quarantine is a recommendation in the manifest. Nothing in this PR hides or moves those rows. A legacy row that already has `tenant_id` from the earlier default backfill is still visible to that stored tenant.
- An empty successful scan does not delete the last namespaced document. Health can still report it missing.
- A failed sync may have upserted earlier files in the same tenant before the error. It does not report success and it does not delete.
- Write-scoped API keys are not vault readers under the current matrix. Fixing that would be new RBAC.
- No background vault job exists. `Sync()` is closed so a later job cannot call the unscoped reconcile.
- CI runs the Go suite with ephemeral Postgres. That is not browser qa:tabs and not UI end-to-end proof.

## Tests

`go test ./...` with `GOSO_TEST_POSTGRES_DSN` set (CI service `postgres:16`). Gateway, control-plane, desktop, `make verify`, and `make smoke` as in `.github/workflows/ci.yml`.

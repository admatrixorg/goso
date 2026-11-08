# SPEC 149 — Vault scoped storage and sync

Status: implemented on `cursor/vault-scoped-storage-sync-e110`.
Plan item: owner roadmap **01 · vault-scoped-storage-sync** (not a SPEC id).
Backend-only. No product UI change. The all-tab browser check is still required; a screenshot is not a substitute for it.

Sample data is Minh An only (sales / pháp chế). This spec does not read customer data and does not copy GoClaw. It does not add staff identity or per-document RBAC.

The phrase for this capability in new text is **truy vấn kiến thức và dữ liệu xuyên phòng ban**.

## Number

Checked before assigning 149:

| Number | Where it is used |
|---|---|
| 001–133 | Files already on `main` under `.planning/specs/` (with the historical gaps 015, 021, 022, 025, 026, 028, 030–032, 081). Highest file is `133-chat-agent-model.md`. |
| 134–139 | Reserved by name in `.planning/specs/goso-2026-10-31-queue.md` (memory, connector, project, approval, meeting, entity). |
| 140 | Open PR #24, branch `cursor/jev-knowledge-filter-f553`: `.planning/specs/140-jev-knowledge-filter.md` and `docs/qa/140-jev-knowledge-filter.md`. |
| 141–148 | Named as the retired 134–148 sequence in that same queue file (“do not implement it in parallel”). |
| 149 | Not a file on `main`, not named in the queue, not in open PRs #10, #23, or #24. |

Open PRs checked with `gh pr list --state open`: #10, #23, #24, #25. Only #24 and this PR touched a spec number, and #24 owns 140.

## Why

Vault sync walked one shared root and reconciled the whole registry. Lookups and the SQLite update were not tenant-scoped. Postgres `vault_docs.path` was unique for the whole table. That is a source-level risk, not a confirmed production exploit.

A stored `tenant_id` is not proof of an owner. The older tenant migration filled an empty tenant with `default`. `owner_explicit` stays 0 unless this process recorded a resolved tenant on a new write. Those legacy rows are not served.

## Goal

One tenant cannot read, write, or sync-delete another tenant’s knowledge, even when the title and logical path match. Unclear-owner rows stay on disk and in the registry, and they are absent from retrieval. Sync reconciles only the caller’s explicit files. Inventory is a dry-run recommendation and is incomplete.

## Behavior

1. Logical `path` is unique per `tenant_id` on memory, SQLite, and Postgres. New files go under `GOSO_VAULT_DIR/t/<tenant>/`. That directory is assigned from the resolved tenant. Path segments are not agent, project, or customer grants. A symlink at `t/`, at `t/<tenant>`, or at a nested directory is rejected in put, get, sync, health, and inventory.
2. Put publishes the file only after the registry write commits. A database error leaves the previous file bytes in place. Put by title and sync by path never change a row whose `owner_explicit` is false.
3. Get, list, search, links, backlinks, graph, health, and counts omit unclear-owner rows entirely: no body and no id, title, or path. Search applies that predicate before rank and `LIMIT`.
4. `POST /api/vault/sync` calls `SyncTenant` for `tenant.Resolve` only. Forged query or body fields do not select a tenant. `Sync()` with no tenant always errors.
5. Flag **vault_scoped_sync** is `GOSO_VAULT_SCOPED_SYNC`. Default off. Off refuses sync and does not restore a global scan. Off does **not** roll the schema back.
6. Deletes run only after a finished, non-empty scan, and only for an explicit row whose file was verified absent. A skipped symlink or invalid path is not an authoritative absence, so that row is not deleted. An empty scan does not delete. The last missing file stays in the registry, but get and search do not present the cached body as current (`body_status=unconfirmed`, body and snippet empty).
7. `GET /api/vault/inventory` is the only inventory method. `apply=1` is ignored. There is no POST. The report lists the resolved tenant’s registry rows (`mapped` / `quarantine`) and files inside that tenant directory. It does **not** list the shared root. This inventory is **incomplete**. It is not a finished migration and not a finished quarantine.

## Permission

No new roles. The existing admin, view, and API-key matrix is unchanged, and read is not widened for write keys.

- The existing admin bearer (`GOSO_ADMIN_TOKEN`) is the principal that may GET the inventory dry-run. An issued API key whose scopes already include `admin` may also GET it, because that scope already implies every route. No new scope was added.
- `X-Goso-Tenant` is a tenant selector for that admin token when `GOSO_MULTI_TENANT=1`. It is not a staff id.
- Unauthenticated callers, the view token, and issued keys scoped only `read` or only `write` are denied on inventory GET and POST. A deny body contains no document id, title, path, or body.
- `write` may still PUT and POST other non-privileged vault routes. It still cannot GET them, including inventory.

## Migration and schema rollback

Additive `owner_explicit INTEGER NOT NULL DEFAULT 0`. Existing rows stay 0. SQLite rebuilds `vault_docs` inside one transaction when `path` is still globally unique, copies rows, and does not move files. A fault before commit rolls the rebuild back; opening again completes it; opening a second time does not duplicate rows. Postgres applies the column, the drop of `vault_docs_path_key`, and the `(tenant_id, path)` unique index in one transaction.

Turning the sync flag off does not reverse that schema. Rolling back the binary leaves the column and the new unique index in place. That is the schema rollback: do not `DROP TABLE`, `DROP COLUMN`, or delete rows to undo this spec. The data stays.

## Rollback of sync

Turn `GOSO_VAULT_SCOPED_SYNC` off. New sync calls fail closed. Unclear-owner rows stay unserved. There is no return to a global delete.

## Non-goals

Converters, PARA, taxonomy, AI, moving legacy files, assigning owners, deleting unclear-owner docs, applying quarantine, staff RBAC, and per-document ACL.

## Tests

`TestB1_UnclearOwnerDeniedBodyAndMetadata` (memory, SQLite, Postgres), `TestB2_InventoryDenyLeaksNoMetadata`, `TestB3_SQLiteMigrationFaultIsReplaySafe`, `TestB3_PostgresMigrationFaultRollsBack`, `TestB4_TwoServicesSameTenantDoNotDropPut`, `TestB4_TwoHTTPRequestsSameTenant`, `TestB4_DBFailureDoesNotPublishFile`, `TestB5_NamespaceSymlinkRejected`, `TestB5_ParentAndNestedSymlink`, `TestEmptyScan_DoesNotPresentCachedBody`, `TestPartialSkip_DoesNotAuthoritativelyDelete`, plus the earlier tenant-isolation tests.

`go test ./...` with `GOSO_TEST_POSTGRES_DSN`. CI service `postgres:16`. `make verify`, desktop verify, `make smoke`, and `npm run qa:tabs` against a local gateway and control plane.

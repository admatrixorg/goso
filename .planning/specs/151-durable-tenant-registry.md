# SPEC 151 — Durable tenant registry

Status: implemented on `cursor/durable-tenant-registry-f110`.
Backend-only. No control-plane tab change. The all-tab browser check is still required; a screenshot is not a substitute for it.

Sample data is Minh An only (sales / pháp chế). This spec does not read customer data and does not copy GoClaw.

## Number

Checked before assigning 151:

| Number | Where it is used |
|---|---|
| 001–133 | Files already on `main` under `.planning/specs/` (historical gaps remain). |
| 134–139 | Reserved by name in `.planning/specs/goso-2026-10-31-queue.md`. |
| 140 | Open PR #24. |
| 149 | On `main`: `.planning/specs/149-vault-scoped-storage-sync.md`. |
| 150 | Open PR #35, branch `cursor/context-files-store-8978`: `.planning/specs/150-context-files-store.md`. |
| 151 | Not a file on `main`, not named in the queue, not used by open PRs #10, #23, #24, or #35. |

## Why

`gateway/internal/tenant/registry.go` kept tenant rows in a process map. `POST /api/tenants` (create, status, members) disappeared when the gateway process exited. `New()` recreated master only. A restart on a host that already uses the goso database (SQLite file or Postgres) dropped every tenant except master.

## Goal

Tenant lifecycle rows survive a process restart in the existing goso database, for both SQLite and Postgres. Master stays present. An empty table behaves as today's startup.

## Schema

Add-only. `migrateTenantRegistry` runs in one transaction at the end of the SQLite and Postgres migrators. It does not `ALTER`, rebuild, or rewrite existing tables or rows.

- `tenant_registry(id, name, status, master, created_at)` — `id` is the slug.
- `tenant_registry_members(id, tenant_id, subject, role, created_at, pos)` — `pos` keeps member order. `subject` is the label the registry already stores. There is no token column.

A fault before commit rolls the new tables back. Opening again creates them. Opening a second time does not duplicate rows. Memories, vault documents, and contacts written before the new migration are still readable after it.

## Behavior

1. On startup the gateway loads every persisted row into the registry. Master is always present. If the table has no master row, master is the same in-memory row `New()` builds today (name `Master`, status `active`, no members, a fresh `created_at`). A stored master row keeps its name, `created_at`, and members, and stays active. A stored row that breaks the registry rules fails startup. The error text is `tenant registry load failed`. The log line is `goso tenant registry: load failed class=<class>`.
2. Create, status, member add, member role, and member remove write the tenant row and its full member list in the same database transaction, then update the cache. If the write fails, the method returns `ErrStore` and the cache is unchanged. HTTP maps that to 500 `tenant registry save failed`. The log line is `goso tenant registry: <op> failed class=<class>` with `op` in `create`, `status`, `member_add`, `member_role`, `member_remove` and `class` in `closed`, `conflict`, `schema`, `driver`, `store`. The driver text, slug, name, and member label are not logged.
3. Unset `GOSO_DB_PATH` and `:memory:` still use the process-local registry. That mode has no database to reopen, which is the same limit agents already have. A SQLite file and Postgres both persist.

## Flag

No flag. An empty `tenant_registry` loads as master-only, which is today's startup. Turning persistence off would keep the bug. The safe default is to write through whenever the gateway opened SQLite or Postgres.

## Semantics that stay

- `maxTenants` (256) and `maxMembers` (64).
- `ErrExists` when the slug is already registered.
- `ErrMaster`: master cannot be deactivated, including after a reload.
- A deactivated tenant rejects writes. An id that is not registered stays writable and is not listed (header isolation).
- GET bodies have no secret fields. A view token still cannot write.
- Memory, vault, and contacts stores are not modified. Their tables are not altered by this migration.

## Tests

Minh An rows only (`minh-an`, `phap-che`, `kinh-doanh`, `covan.phaply@minh-an.test`).

- Create tenants, add members, change a role, remove a member, deactivate one tenant, add a master member. Close the store, open it again, load a new registry. Ids, status, roles, and `created_at` match. Master cannot be deactivated. An unknown slug is still writable and is not listed.
- Close the database, then create, add a member, and change status. Each call returns an error, the cache is unchanged, the log has op and class only, and a new registry on the same database does not show the failed write.
- An empty table reloads master with a new `created_at` and no other tenants.
- Migration twice, a faulted migration that rolls back, and a memory note, vault doc, and contact that survive the new tables. SQLite always. Postgres when `GOSO_TEST_POSTGRES_DSN` is set.

## Non-goals

A feature flag, changes to the tenants page, staff RBAC, and copying another product's tenant table.

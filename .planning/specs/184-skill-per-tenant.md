# SPEC 184 — Per-tenant skill directory and loader

Status: runtime only. One PR into `main`. Do not merge from this file.

SPEC 175 (PR #48) keeps `--apply` refused. Turning that flag on is a later PR. This spec does not add a skill table, does not execute skill scripts, and does not change the control plane.

Sample data is Minh An only (`minh-an-sales` / `Cố vấn Minh An`). No customer dump. No GoClaw or Dewee source.

## Why

`GOSO_SKILLS_DIR` is one shared folder. `skill.List` and `skill.Load` do not take a tenant, and BM25 keeps a single `liveIdx`. A tenant that can create a skill can overwrite a name every other tenant reads.

## Behavior

1. Shared skills stay at `<dir>/<name>/SKILL.md`. The `default` tenant is the only tenant that may Create or Delete there. Every other tenant reads those files and must not write or remove them.
2. A tenant's own skills live at `<dir>/t/<tenant>/<name>/SKILL.md`. The `t` segment matches `vault/scope.go` so a tenant id cannot collide with a shared skill name. `validName` reserves `t`: no skill folder may be named `t`.
3. The tenant directory is checked the way `vault/scope.go` checks `tenantDir`: `store.TenantOK`, then reject `..`, `/`, and `\`, then `rejectSymlinks`, then `underRoot`. `store.TenantOK` alone is not enough, because it accepts `..`. An empty tenant is `default`. `t/default` is not a second store for the default tenant.
4. List, Load, and Search for a non-default tenant are the shared root plus that tenant's own folder. The same name in the tenant folder wins. The default tenant sees and searches the shared root only.
5. Create writes a temp file in the already-checked skill folder (`O_EXCL`, no follow), fsyncs it, and renames it onto `SKILL.md`. A symlink at the destination is not replaced and is not written through. Cap remains 64KiB. Scripts are never executed.
6. `GET`/`POST`/`DELETE /api/skills` use `requestTenant`. `use_skill` and `skill_search` take the tenant from `agent.TenantID`, the same lookup as memory and session tools, and use `default` when there is no agent.
7. Import/export `Catalog`, `Export`, `Import`, `Rollback`, and `skillSet` use that tenant's visible set and write only that tenant's directory (`default` still writes the shared root). Rollback removes or restores only that tenant's files.
8. BM25 keeps one index per skills root and tenant. A search does not rank another tenant's private files, and it does not share one global `liveIdx`.

## Non-goals

`goso-skill-import --apply`, moving files that are already on disk, a skill database, script exec, pgvector, and any control-plane change.

## Tests

Path traversal (`..`, `/`, `\`), a symlink at `t/` or `t/<tenant>/`, reserved name `t`, shared read-only override, a non-default tenant blocked from creating or deleting a root skill, per-tenant BM25 isolation, `use_skill` / `skill_search` agent scope with a default fallback, and import/export rollback scoped to `t/<tenant>/`.

`go test ./...` and `go test -race` on the skill, builtin, agent, httpapi, impexp, and pipeline packages. `npm run qa:tabs` on the code commit. Log: `docs/qa/184/qa-tabs-<code_sha>.log`.

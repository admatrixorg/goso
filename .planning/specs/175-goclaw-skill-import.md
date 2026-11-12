# SPEC 175 — GoClaw skill import

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw. Skill text is data. The importer never executes it.

## Why

GoClaw keeps skills in `skills`, with `skill_versions`, `skill_agent_grants`, `skill_user_grants`, and `skill_tenant_configs`. Goso already stores a skill as one folder under `GOSO_SKILLS_DIR`: `<name>/SKILL.md`. The folder name matches `^[a-z0-9_-]{1,64}$` (`gateway/internal/skill/skill.go`). There is no skill table, no version table, and no grant table. This PR fills the folders that already exist. It does not add a second schema.

## Command

`goso-skill-import` reads one export file. It does not open a database, `GOSO_DATABASE_URL`, or a live GoClaw connection.

```bash
go run ./gateway/cmd/goso-skill-import \
  --export export.json \
  --tenant minh-an \
  --skills-dir /path/to/skills \
  --src-dir /path/to/files \
  --rename-map /path/to/skill-rename-map.json
```

`--apply` writes. Without it, the run is a dry-run. `--apply` together with `--dry-run` is an error. `--dry-run=false` without `--apply` still writes no skill files.

Dry-run prints per-tenant counts and the rename map on stdout. The only file it writes is `--rename-map`, and that path must sit outside `--skills-dir`. It does not create the skills directory, the source-id file, or any `SKILL.md`.

`--apply` checks that `--rename-map` matches the names computed for this export and this skills directory. A missing file is written, then the new folders are created. A different file is an error and creates nothing. A second apply of the same export does not add a second folder and does not change bytes that are already there.

## Rename rules

The old name is `slug` when that column is non-empty, otherwise `name`.

1. `đ` / `Đ` become `d`.
2. Combining marks are stripped (`Tư vấn` becomes `Tu van`).
3. The string is lowercased.
4. Every run that is not `a-z` or `0-9` becomes one hyphen. `_`, spaces, and `+` are hyphens.
5. Leading and trailing hyphens are removed.
6. The result is cut to 64 characters.
7. An empty result is `invalid-name` and is not written.
8. Inside the selected tenant, rows are ordered by source id. The first keeps the base name. The next free name is `base-2`, `base-3`, still at most 64 characters.
9. A folder that already exists, or a name already recorded for another source id, is not overwritten. The new skill takes the next free suffix. A pre-existing `san-co` stays `san-co`. The imported row becomes `san-co-2`.

`Bao_Gia_Minh_An` becomes `bao-gia-minh-an`. `Tư vấn + Hóa đơn` becomes `tu-van-hoa-don`. `web_search-patterns` becomes `web-search-patterns`.

Grants address a skill by `skill_id`. The rename map rewrites that id to the old name and the new folder name. On apply, the folders use those new names.

## Column mapping

| GoClaw | Goso |
|---|---|
| `skills.id` | source-id file only (`.goso-skill-import.json`) |
| `skills.tenant_id` | selects the run; stored on the source-id row. Not a folder field |
| `skills.slug`, else `skills.name` | folder name after the rename rules |
| `skills.file_path` | path under `--src-dir`. A directory reads only `SKILL.md` inside it |
| `skills.version` | the file at `file_path` is the current file. The integer is not stored |
| `skills.status` | only `active` (empty counts as active). Anything else is `inactive` |
| `skills.enabled` | `false` is `disabled` and is not written. Missing means enabled |
| `skills.is_system` | `true` is not written |
| `skill_tenant_configs.enabled` | `false` for that skill and tenant skips the file. The row is not stored |
| `skill_agent_grants`, `skill_user_grants` | not stored. Old name and new name are written in the rename map |
| `skill_versions` | not stored. Counted per tenant |

The body is the file bytes. Newlines are normalized to `\n`. The file is not executed. A sibling script is not read and not run. Over 64KiB (`skill.MaxBody`) is `too-large` and is not truncated. Empty is `empty-body`. NUL or non-UTF-8 is `binary-skipped`. `..`, an absolute path, or a symlink is not followed.

## Not imported, needs a decision

Goso's skill folder has no field for these. They are counted and dropped. This PR does not add tables for them.

- `skill_versions` (there is no content column and no version table; history is not kept)
- `skill_versions.changed_files`, `skill_versions.content_hash`, actor columns, `created_from_suggestion_id`
- `skill_agent_grants` and `skill_user_grants` as stored rows (`pinned_version`, `can_manage`, `agent_id`, `user_id`, `granted_by` have nowhere to go)
- `skill_tenant_configs` as a stored switch (only `enabled=false` changes this run, by skipping the file)
- `skills.embedding` (no vector is written)
- `skills.owner_id`, `skills.visibility`, `skills.tags`, `skills.deps`, `skills.frontmatter`
- `skills.description` (the file body is the text; the column is not copied in)
- `skills.file_hash`, `skills.file_size`, `skills.created_at`, `skills.updated_at`

`is_system=true` skills are skipped on purpose. Issue 28 says those six built-ins are not part of the transfer.

One `GOSO_SKILLS_DIR` is shared by every tenant. This PR does not add a per-tenant skill directory. A run writes only `--tenant`. Another tenant's rows are `foreign-tenant`. A later run for that other tenant cannot replace a folder this source id does not own.

## Denylist

`--deny-tenants` or `GOSO_IMPORT_DENY_TENANTS` is a local file, one tenant id per line. The file is required. Missing, empty, and malformed files stop the process before a dry-run and before any write. If `--tenant` or `--source-tenant` is listed, the run writes nothing. The report field `denied_tenants` is a count. The names in the file are not printed.

## Apply

`GOSO_SKILLS_DIR` is one directory. `skill.List` and `skill.Load` do not take a tenant, so a folder written for one tenant is visible to every tenant. `--apply` returns an error and writes no files. The folder writer stays in the package for a later per-tenant skills spec. It is not called by `--apply`.

## Rollback

`--rollback` counts folders recorded in `.goso-skill-import.json` for the exact `--tenant`. `--rollback --apply` removes those folders and those map rows. A folder that is not in the map, and a folder recorded for another tenant, stay.

## Existing folders

When the destination folder already exists and the bytes differ, the plan reason is `native-exists`. The file is not updated. A second skill is not created beside it.

## Report

Each tenant in the file has `skills_input = inserted + skipped + rejected`. Version rows are `versions_not_imported`. Grant rows are `grants_not_imported`. A grant whose skill is written or already owned counts as `grants_rewritten` because the map carries the new name. Config rows are `configs_not_imported`.

Stdout and the log do not include the skill body, an embedding, `owner_id`, `agent_id`, `user_id`, or `granted_by`.

## Tests

`gateway/internal/skillimport` and `gateway/cmd/goso-skill-import`. The fixture is `testdata/minh-an.json` plus fake files under `testdata/src`. Tenants are `minh-an` and `khac`. No customer export.

## Non-goals

No control-plane change. No new skill table. No pgvector. No copy of GoClaw. No live database. Memory, vault, contacts, and context files are not written.

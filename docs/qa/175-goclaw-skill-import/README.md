# SPEC 175 — GoClaw skill import

Offline importer for one tenant into the existing skill folders. Sample data is Minh An. No customer data and no secret values.

| Flag | Meaning |
|---|---|
| `--export` | JSON object or JSONL. Columns match `skills`, `skill_versions`, `skill_agent_grants`, `skill_user_grants`, `skill_tenant_configs` |
| `--source-tenant` | Export `tenant_id` to plan. Required |
| `--tenant` | Tenant id checked against the denylist. Rollback uses this id |
| `--deny-tenants` | Local denylist file. `GOSO_IMPORT_DENY_TENANTS` is the same path when the flag is empty |
| `--skills-dir` | Goso skills directory. One folder per skill |
| `--src-dir` | Files named by `file_path` |
| `--rename-map` | Old-to-new names. Outside `--skills-dir`. Written by a dry-run |
| `--apply` | Refused for import. With `--rollback`, removes that tenant's mapped folders |
| `--rollback` | Count mapped folders for `--tenant`. Add `--apply` to remove them |

Dry-run writes the rename map and no skill file. `--apply` returns an error and writes no files, because the skills directory is shared. A local denylist file is required. A missing, empty, or malformed file exits non-zero before a dry-run. A listed tenant writes nothing. The report field `denied_tenants` is a count and does not name the tenant.

`--rollback` counts mapped folders for `--tenant`. `--rollback --apply` removes only those folders. A native folder and another tenant's folder stay.

```bash
go test -race -count=1 -timeout 180s ./gateway/internal/skillimport ./gateway/cmd/goso-skill-import
go test ./... -count=1 -timeout 25m
```

Both exited 0 on this branch. `GOSO_TEST_POSTGRES_DSN` was unset, so existing Postgres cases skipped.

The all-tabs log is [qa-tabs-1db17d6f8ce8f9589f3fe7a573e0b081513fd60a.log](../175/qa-tabs-1db17d6f8ce8f9589f3fe7a573e0b081513fd60a.log). The browser ran against code commit `1db17d6f8ce8f9589f3fe7a573e0b081513fd60a`. Gateway and control plane were local. Seed agent `minh-an-sales` / `Cố vấn Minh An` / `echo`. Exit 0. Last lines: `flow chat` then `qa:tabs ok`.

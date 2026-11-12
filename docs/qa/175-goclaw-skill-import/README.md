# SPEC 175 — GoClaw skill import

Offline importer for one tenant into the existing `SKILL.md` folders. No control-plane change, so prototype screenshots do not apply and `npm run qa:tabs` was not run. No customer data and no secret values.

| Flag | Meaning |
|---|---|
| `--export` | JSON object or JSONL. Columns match `skills`, `skill_versions`, `skill_agent_grants`, `skill_user_grants`, `skill_tenant_configs` |
| `--tenant` | Export `tenant_id` to write. Other tenants are counted and not written |
| `--skills-dir` | Goso skills directory. One folder per skill |
| `--src-dir` | Files named by `file_path` |
| `--rename-map` | Old-to-new names plus rewritten grant names. Outside `--skills-dir` |
| `--apply` | Create new folders. Absent means dry-run |

Dry-run writes the rename map and no skill file. The skills directory is unchanged. `--apply` refuses a rename map that does not match this export. A second apply does not duplicate or overwrite.

The sample is `gateway/internal/skillimport/testdata/minh-an.json`.

```bash
go run ./gateway/cmd/goso-skill-import \
  --export gateway/internal/skillimport/testdata/minh-an.json \
  --tenant minh-an \
  --skills-dir /tmp/minh-an-skills \
  --src-dir gateway/internal/skillimport/testdata/src \
  --rename-map /tmp/minh-an-skill-rename.json
```

## Tests

```bash
go test -race -count=1 -timeout 180s ./gateway/internal/skillimport ./gateway/cmd/goso-skill-import
go test ./... -count=1 -timeout 25m
```

Logs: [go-test-race.txt](go-test-race.txt), [go-test.txt](go-test.txt). Both commands exited 0. `go build -o /tmp/goso-skill-import ./gateway/cmd/goso-skill-import` exited 0.

`GOSO_TEST_POSTGRES_DSN` was unset for the full run, so existing Postgres cases skipped. The new package does not open Postgres and does not create a vector column.

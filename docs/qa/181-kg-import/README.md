# SPEC 181 — KG import

Offline import of a JSONL export into the existing `kg_entities` and `kg_relations` tables. No UI change, so prototype screenshots do not apply. No customer data and no secret values. The Postgres DSN is named, not filled in.

The tool never opens a GoClaw database. `--export` is JSONL. `--db` is a local SQLite file or a loopback Postgres DSN.

## Flags

| Flag | Meaning |
|---|---|
| `--export` | JSONL. Each line is `kg_entities` or `kg_relations` with the GoClaw column names. `embedding` and `tsv` are dropped |
| `--source-tenant` | GoClaw tenant for this run. Required |
| `--tenant` | Goso tenant to write. Required |
| `--tenant-map` | SPEC 152 pairs. `source_tenant`, `dest_tenant`, `slug`. The run writes only `--source-tenant` into `--tenant` |
| `--customer-map` | Optional PR #46 file. `{channel, sender_id, user_id}` becomes `<channel>:<id>`, else `goclaw:<user_id>` |
| `--deny-file` | Required local denylist. `GOSO_TENANT_DENY_FILE` is the fallback. Missing, empty, or malformed stops before any work. A listed source or dest stops before any write. The report field `denied_tenants` is a count |
| `--agent-map` | SPEC 174 pairs. `goclaw_agent_id`, `goso_agent_id`, optional `source_tenant`. The Goso agent must already exist in the destination tenant |
| `--db` | Local SQLite file or loopback Postgres DSN. `GOSO_DATABASE_URL` is not opened |
| `--dry-run` | Counts only. `read = will_write + sum(skipped)` per source tenant and table |
| `--remove` | With `--tenant`, delete `gc-` rows for that tenant. The denylist applies |

Skipped keys: `expired` (stays 0; a past `valid_until` is kept), `customer-private`, `team/scope`, `unmapped`, `missing relation endpoint`, `exists`, `native-exists`, `invalid`.

```bash
go run ./gateway/cmd/goso-kg-import \
  --export export.jsonl \
  --tenant-map tenants.json \
  --agent-map agents.json \
  --source-tenant <goclaw-tenant> \
  --tenant minh-an \
  --deny-file /path/to/local-deny.txt \
  --db /path/to/local.db \
  --dry-run
```

```bash
go run ./gateway/cmd/goso-kg-import \
  --remove --tenant minh-an \
  --deny-file /path/to/local-deny.txt \
  --db /path/to/local.db
```

## Maps

Tenant file:

```json
{"tenants":[{"source_tenant":"<goclaw tenant uuid>","dest_tenant":"minh-an","slug":"minh-an"}]}
```

Agent file. This PR does not read the agents-importer branch. SPEC 174 is expected to be able to emit this file.

```json
{"agents":[{"goclaw_agent_id":"<uuid>","goso_agent_id":"<goso id>","source_tenant":"<goclaw tenant uuid>"}]}
```

## What was run

From the repo root, on this branch, with `GOSO_TEST_POSTGRES_DSN` unset (no local Postgres server in this environment):

```bash
go build -o /tmp/goso-kg-import ./gateway/cmd/goso-kg-import
go test ./gateway/internal/kgimport ./gateway/internal/store ./gateway/cmd/goso-kg-import -count=1 -timeout 180s
go test -race -count=1 -timeout 240s ./gateway/internal/kgimport ./gateway/internal/store ./gateway/cmd/goso-kg-import
```

Exit 0. Memory and SQLite ran `TestMinhAnFixture`, `TestConflictDoesNotOverwrite`, `TestDeleteImportedKGLeavesLiveRows`, `TestDenyFileAndDeniedTenant`, and the CLI test. The Postgres subtest skips when `GOSO_TEST_POSTGRES_DSN` is unset, and fails in CI when that variable is missing. No customer export is in this directory.

## qa:tabs

Log for code commit `764015195370d514859a2a312d3dd5b2efc1247a`: [qa-tabs-764015195370d514859a2a312d3dd5b2efc1247a.log](../181/qa-tabs-764015195370d514859a2a312d3dd5b2efc1247a.log).

Chrome binary `/usr/local/bin/google-chrome`. Gateway from that commit, fresh SQLite `GOSO_DB_PATH`, `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` development. Seed agent `minh-an-sales` / `Cố vấn Minh An` / `echo`. Exit 0. Last lines: `flow chat` then `qa:tabs ok`.

## Column assumptions that are not in the `\d`

The `\d` of 2026-11-12 is the column list. These are the file and mapping choices around it:

- JSONL, one object per line, with a `table` field. That is this tool's export, not a `pg_dump`.
- JSON `null` `user_id` is treated as empty.
- `kg_relations` has no description, so the relation body is compact `properties` only.
- A relation id is `gc-kg:<relation uuid>`. Endpoint ids are `gc-kg:<entity uuid>`.
- `GetKGEntity` still classifies an unknown provenance as `posted`. The column value is `goclaw:<uuid>`.

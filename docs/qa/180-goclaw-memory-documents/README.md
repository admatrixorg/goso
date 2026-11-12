# SPEC 180 — GoClaw memory_documents

Offline import of `memory_documents` from a JSONL file. No UI change. No customer export and no secret values. The Postgres DSN is not stored here.

The fixture is Minh An only (`minh-an`, `phap-che`, synthetic `khach-x` / `khach-y` / `khach-z`). `mq-capital` is a skipped slug in that fixture, not a copy of that tenant.

## Command

```text
goso-memdoc-import --export file.jsonl --map map.json --db path --vault-dir dir [--dry-run]
goso-memdoc-import --remove-tenant <goso tenant> --db path --vault-dir dir
```

`--map` is JSON. `tenants` is the SPEC 152 source id → dest tenant pair, plus `slug`. `agents` is the GoClaw agent id → existing Goso agent id. `customers` is optional; a missing user becomes `goclaw:<user_id>`.

## go test

`GOSO_TEST_POSTGRES_DSN` was unset here, so the Postgres case skipped. The memory store and SQLite ran. CI sets that DSN.

```bash
go build -o /tmp/goso-memdoc-import ./gateway/cmd/goso-memdoc-import
go test ./... -count=1 -timeout 25m
go test -race ./gateway/internal/memdocimport ./gateway/cmd/goso-memdoc-import ./gateway/internal/store ./gateway/internal/vault -count=1 -timeout 300s
```

All three exited 0. `go test ./...` reported `ok` or `[no test files]` for every package. The race run covered the importer, the command, the store, and the vault.

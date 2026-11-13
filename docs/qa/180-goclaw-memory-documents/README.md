# SPEC 180 — GoClaw memory_documents

Offline import of `memory_documents` from a JSONL file. No UI change. No customer export and no secret values. The Postgres DSN is not stored here.

The fixture is Minh An only (`minh-an`, `phap-che`). The denylist in tests uses a fake tenant token, not a production name.

## Command

```text
goso-memdoc-import --export file.jsonl --map map.json --tenant <source> --deny-file tenants.txt --db path --vault-dir dir [--customer-map contacts.json] [--system-names-out names.txt] [--dry-run]
goso-memdoc-import --remove-tenant <goso tenant> --deny-file tenants.txt --db path --vault-dir dir
```

`--map` is JSON. `tenants` is the SPEC 152 source id → dest tenant pair, plus `slug`. `agents` is the GoClaw agent id → existing Goso agent id. `--customer-map` is the contacts importer TSV (`channel`, `sender`, `user`, `key`). Channel names are normalized (`zalo_personal` → `zalo-personal`, `lark` → `feishu`). A user with no row becomes `goclaw:<user_id>`.

## qa:tabs

Code under test: `b8da4a398068f70006c8369be01b597a5b6c26f2`.

Log: [qa-tabs-b8da4a398068f70006c8369be01b597a5b6c26f2.log](../180/qa-tabs-b8da4a398068f70006c8369be01b597a5b6c26f2.log).

Chrome: **Google Chrome 156.0.8078.47**. Binary: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set. `QA_SHOTS` was not set.

From `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from this tree, at that commit. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

## go test

`GOSO_TEST_POSTGRES_DSN` was unset here, so the Postgres case skipped. The memory store and SQLite ran. CI sets that DSN.

```bash
go build -o /tmp/goso-memdoc-import ./gateway/cmd/goso-memdoc-import
go test ./... -count=1 -timeout 25m
go test -race ./gateway/internal/memdocimport ./gateway/cmd/goso-memdoc-import ./gateway/internal/store ./gateway/internal/vault -count=1 -timeout 300s
```

All three exited 0. `go test ./...` reported `ok` or `[no test files]` for every package. The race run covered the importer, the command, the store, and the vault.

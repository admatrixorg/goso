# Durable contacts store

Evidence for the tenant-scoped contacts tables. No customer data and no secret values. Env vars are named, not filled in. Fixtures in the tests are synthetic Minh An rows.

## Tested SHA

`aa954f254c40be6754540f2d54c4095d2c6daf02`

`git status --porcelain` was empty before both commands. This directory is the only change after that SHA. The stdout does not print a later SHA.

Postgres for the local `go test` was **16.15** (Ubuntu package). The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## Gateway tests

[go-test-aa954f254c40be6754540f2d54c4095d2c6daf02.txt](go-test-aa954f254c40be6754540f2d54c4095d2c6daf02.txt)

From `gateway/`:

```bash
go test ./... -count=1 -timeout 300s
```

Exit 0. Every package reported `ok` or `[no test files]`.

Contacts cases in that run:

- more than 200 rows, read back, on SQLite and Postgres
- close and reopen keeps the rows and the same ids
- tenant isolation on the store (list, get, count, search, merge) on both backends
- bulk upsert twice, and a duplicate key inside one batch, leaves one row per natural key
- flag off matches a pure in-memory directory, including the 200 cap, and writes nothing
- HTTP cap with the flag off, HTTP list of 205 with the flag on
- HTTP tenant isolation with the flag off and on, including a view token that cannot switch tenants
- migration twice, and a faulted migration that rolls back, on both backends
- a memory note, a vault doc, a KG entity, and a KG relation survive the contacts migration

## qa:tabs

[qa-tabs-aa954f254c40be6754540f2d54c4095d2c6daf02.log](qa-tabs-aa954f254c40be6754540f2d54c4095d2c6daf02.log)

Chrome: **Google Chrome 156.0.8078.47**.

From `control-plane/`, `QA_SHOTS` unset:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=$(command -v google-chrome) npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway` on `127.0.0.1:8080` with a fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Env names set: `GOSO_DEV_MODE`, `GOSO_RATE_LIMIT` (0), `GOSO_ENV` (`development`). Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_CONTACTS_DURABLE`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped. `#/contacts` passed. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

This change does not alter the control plane. Prototype screenshots are not part of this run.

## Natural key

A sighting is `(tenant_id, channel, dest)`. `dest` is goso's channel-external id (peer, group chat, or a sender id stored as its own row). The in-memory index already uses `tenant|channel|dest`. After a merge, one contact owns many of those pairs, so the unique key is on `contact_identifiers`, not on the contact row alone. Two tenants may share a channel and dest.

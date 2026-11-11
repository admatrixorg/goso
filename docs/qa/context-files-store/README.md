# SPEC 150 — context files store

Transcripts after the owner cap, the view-token write check, and the merge of `main` at `acd85f1`. No customer data and no secret values. The Postgres DSN is not stored here.

No control-plane tab changed, so there are no prototype-versus-Goso screenshots.

## go test

`go test ./... -count=1` from the repo root.

| Log | Postgres | Result |
|---|---|---|
| [go-test.txt](go-test.txt) | `GOSO_TEST_POSTGRES_DSN` unset. Postgres cases skip. CI sets the DSN. | exit 0 |
| [go-test-postgres.txt](go-test-postgres.txt) | Local Postgres **16.15**. `CI=true` and `GOSO_TEST_POSTGRES_DSN` set (same user, database, and host shape as `.github/workflows/ci.yml`). | exit 0 |

Both runs finished with `EXIT:0`. No `FAIL` lines. The suite includes the Minh An owner-cap cases (17th file and a byte total over 64KiB write nothing; a chat turn over the cap returns `too_large` and does not call the model) and the view-token PUT/DELETE denial.

## qa:tabs

Chrome: **Google Chrome 156.0.8078.47** (`/usr/local/bin/google-chrome`).

Log: [qa-tabs-seeded.txt](qa-tabs-seeded.txt)

`GOSO_CONTEXT_FILES` was unset. `PUT /api/context-files` returned `404` `{"error":"not found"}` before the browser run.

Command, from `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway: `go run ./gateway/cmd/goso-gateway gateway` with `GOSO_DEV_MODE=1`, `GOSO_RATE_LIMIT=0`, `GOSO_ENV=development`, a fresh `GOSO_DB_PATH`, and a fresh `GOSO_VAULT_DIR`. Unset: `GOSO_CONTEXT_FILES`, `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

# GoClaw vault import evidence

Offline importer for one tenant into the existing `vault_docs` and `vault_links` rows. No UI change, so prototype screenshots do not apply. No customer data and no secret values. Env vars are named, not filled in.

## Head

| Commit | What it is |
|---|---|
| `ffa94f9788fbaa5bb46f90bf11605bb5053deb53` | Importer. First `go test ./...` and `qa:tabs` |
| `72808cca31ddae7cae4849983eafc6054c8021af` | Same code plus these notes. Both commands were run again |

`git status --porcelain` was empty before and after each command. The second pair of logs is from `72808cca31ddae7cae4849983eafc6054c8021af`, which does not change gateway or control-plane code relative to the importer commit.

`GOSO_TEST_POSTGRES_DSN` was unset, so existing postgres cases skipped. SQLite and the in-memory store ran. CI sets that DSN.

## go test

Log: [go-test-ffa94f9788fbaa5bb46f90bf11605bb5053deb53.txt](go-test-ffa94f9788fbaa5bb46f90bf11605bb5053deb53.txt)

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. Every package with tests passed, including `gateway/internal/store`, `gateway/internal/vault`, `gateway/internal/vaultimport`, and `gateway/cmd/goso-vault-import`.

## qa:tabs

Chrome: **Google Chrome 156.0.8078.47** (`google-chrome --version`). Binary path: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set. `QA_SHOTS` was not set.

Log: [qa-tabs-ffa94f9788fbaa5bb46f90bf11605bb5053deb53.log](qa-tabs-ffa94f9788fbaa5bb46f90bf11605bb5053deb53.log)

Command, from `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from this tree. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

The same two commands were run again on `72808cca31ddae7cae4849983eafc6054c8021af` with a clean tree, a fresh database, and the same Minh An seed. Exit 0 both times.

- [go-test-72808cca31ddae7cae4849983eafc6054c8021af.txt](go-test-72808cca31ddae7cae4849983eafc6054c8021af.txt)
- [qa-tabs-72808cca31ddae7cae4849983eafc6054c8021af.log](qa-tabs-72808cca31ddae7cae4849983eafc6054c8021af.log)

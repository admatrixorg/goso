# SPEC 149 evidence

Reviewers can read this directory in the PR. It replaces logs that lived only on the agent VM.

No customer data and no secret values. Env vars are named, not filled in.

## Heads

| Commit | What it is |
|---|---|
| `9b1b6f37b0b1d3df043b6edb0769545ec4b0d560` | Pre-fix head the blocker tests failed on |
| `7a3ee90523f3bdd0c53c282791428946127a82e0` | Fixes plus the gitleaks test-marker rename. [Actions run 42491042663](https://github.com/admatrixorg/goso/actions/runs/42491042663) succeeded |
| This evidence commit | Adds this directory and the admin inventory test. Its Actions run is linked from PR #25 |

Postgres for the local `go test` lines was **16.15** (Ubuntu package). CI uses the workflow service image `postgres:16`. The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## qa:tabs

Chrome: **Google Chrome 156.0.8078.47** (`google-chrome --version`). Binary path: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set.

### Seeded branch run (the all-tab result)

Log: [qa-tabs-branch-seeded.log](qa-tabs-branch-seeded.log)

Exit 0. Last lines: `flow chat` then `qa:tabs ok`.

Every sidebar hash, every alias, and the folded routes `#/teams`, `#/heatmap`, `#/marketing` passed. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases. Chat send, agent menu, model menu, `@`, search, rename, archive, and restore passed.

### Empty-database baseline (not skip-passed)

Same command, no agent created first.

| Log | Tree | Result |
|---|---|---|
| [qa-tabs-branch-empty-db.log](qa-tabs-branch-empty-db.log) | this branch | `FAIL chat no agent to start a conversation`. Tabs, aliases, and folded routes passed |
| [qa-tabs-main-bb8cbb5-empty-db-BASELINE-FAIL.log](qa-tabs-main-bb8cbb5-empty-db-BASELINE-FAIL.log) | main `bb8cbb5` | the same single failure |

The empty-DB chat failure exists on main. It was not skipped and not patched.

### QA_SHOTS=1 is not a screenshot-gate pass

[qa-tabs-shots-aborted-NOT-A-GATE.log](qa-tabs-shots-aborted-NOT-A-GATE.log)

`QA_SHOTS=1` stopped with `net::ERR_FILE_NOT_FOUND` on the prototype HTML path the script defaults to. Chat never ran. This is not a pass of the screenshot gate. This PR does not change tab behavior, so prototype screenshots are N/A for that reason. The all-tab gate is the unseeded and seeded `npm run qa:tabs` runs above.

### Commands and env names

Reset (names only):

```bash
rm -f "$GOSO_DB_PATH"
rm -rf "$GOSO_VAULT_DIR"
```

Gateway process env **names**: `GOSO_DEV_MODE` (set to on), `GOSO_RATE_LIMIT` (set to 0), `GOSO_ENV` (`development`), `GOSO_DB_PATH` (fresh file), `GOSO_VAULT_DIR` (fresh directory). Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`.

```bash
go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080
```

Control plane, from `control-plane/`:

```bash
npm run dev -- --host 127.0.0.1 --port 3000
```

Minh An seed (only for the seeded run). Empty-DB runs skip this:

```bash
curl -s -X POST "$GOSO_GW/api/agents" \
  -H 'Content-Type: application/json' \
  -d '{"agent_key":"minh-an-sales","display_name":"Cố vấn Minh An","model":"echo"}'
```

qa:tabs, from `control-plane/`. Env names: `GOSO_UI`, `GOSO_GW`, `CHROME_PATH`.

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Main baseline used the same control plane and the same command, with the gateway built from main `bb8cbb5` and a different fresh `GOSO_DB_PATH` / `GOSO_VAULT_DIR`.

## Blocker tests

Command (env name only):

```bash
GOSO_TEST_POSTGRES_DSN=... go test ./gateway/internal/vault/ \
  ./gateway/internal/httpapi/ ./gateway/internal/store/ \
  -count=1 -timeout 180s \
  -run 'TestB1_|TestB2_|TestB3_|TestB4_|TestB5_|TestEmptyScan_|TestPartialSkip_' -v
```

| File | When |
|---|---|
| [go-test-before-9b1b6f3-vault.txt](go-test-before-9b1b6f3-vault.txt) | Pre-fix `9b1b6f3` plus the regression tests, before the behavior fix |
| [go-test-before-9b1b6f3-http-store.txt](go-test-before-9b1b6f3-http-store.txt) | Same, HTTP and store packages |
| [go-test-after-7a3ee90-vault-http-store.txt](go-test-after-7a3ee90-vault-http-store.txt) | Passing run on the fixed tree `7a3ee90` |
| [go-test-after-head-vault-http-store.txt](go-test-after-head-vault-http-store.txt) | Passing run including the admin inventory allow case |

Before, per backend where the test has one:

- B1 memory, sqlite, postgres: `get served legacy row: <nil>`
- B2: write POST `200` with `path: chinh-sach.md`
- B3 SQLite: `table vault_docs_scoped already exists`
- B3 Postgres: `postgres fault reported success`
- B4 two services: `put finished during scan barrier`
- B4 HTTP: `http put lost 200 []`
- B4 file publish: `file published despite db error`
- B5 namespace symlink: `put followed namespace symlink`
- B5 parent and nested: already PASS on `9b1b6f3` (kept)
- Empty scan: cached body still returned
- Partial skip: `Deleted:1`

After: each of those tests PASS, including B1 `/sqlite`, `/memory`, and `/postgres`.

## Who may GET the inventory

Under the existing contract, unauth, the view token, and issued keys scoped only `read` or only `write` are denied. The dry-run stays usable by the existing admin bearer (`GOSO_ADMIN_TOKEN`) and by an issued key that already has scope `admin` (`Grant.Has("admin")` already allows every route). No grant was added or widened. `TestB2_InventoryDenyLeaksNoMetadata` covers the denies and those two allows.

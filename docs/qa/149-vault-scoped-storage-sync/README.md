# SPEC 149 evidence

Reviewers can read this directory in the PR. It replaces logs that lived only on the agent VM.

No customer data and no secret values. Env vars are named, not filled in.

## Heads

| Commit | What it is |
|---|---|
| `9b1b6f37b0b1d3df043b6edb0769545ec4b0d560` | Pre-fix head the blocker tests failed on |
| `7a3ee90523f3bdd0c53c282791428946127a82e0` | B1–B3 and the SPEC collision, accepted. [Actions run 42491042663](https://github.com/admatrixorg/goso/actions/runs/42491042663) succeeded |
| `55d9b9ab0a6278685f14fd7eb04b629b13f3ec91` | First evidence directory and the admin inventory GET test |
| `57894583ff6b35885ec99968ac76012e8c69e916` | Fail-closed publish and the symlink checks. [Actions run 42492310792](https://github.com/admatrixorg/goso/actions/runs/42492310792) succeeded |
| `5984781feb4e76a857ade2b173d0c03f2da8a186` | Search keeps the snippet hash. The verbose transcripts below that name this tree were run here. Their stdout does not print a SHA |
| `191ba443d0f0acfe41ecb0b00e7e7f6ff41cd11c` | Evidence for the Search race. Also the clean tree under `qa:tabs` after the Search fix |
| `6845ede7e55079625e1301b699b52e2701b0b5b1` | Adds `qa-tabs-final-191ba443d0f0acfe41ecb0b00e7e7f6ff41cd11c-seeded.log`. No code change. qa:tabs was not re-run |

`191ba443d0f0acfe41ecb0b00e7e7f6ff41cd11c` adds the Search transcripts and the corrected B5 note. It does not change gateway code. The Search tests were run on its parent, `5984781`. `6845ede7e55079625e1301b699b52e2701b0b5b1` adds the qa:tabs log for `191ba44` and does not change code either.

Postgres for the local `go test` lines was **16.15** (Ubuntu package). CI uses the workflow service image `postgres:16`. The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## qa:tabs

Chrome: **Google Chrome 156.0.8078.47** (`google-chrome --version`). Binary path: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set.

### Seeded run on the Search-fix tree (the all-tab result)

Log: [qa-tabs-final-191ba443d0f0acfe41ecb0b00e7e7f6ff41cd11c-seeded.log](qa-tabs-final-191ba443d0f0acfe41ecb0b00e7e7f6ff41cd11c-seeded.log)

Tested code SHA, from `git rev-parse HEAD` at 2026-11-08T15:12:20Z, before and after the run: `191ba443d0f0acfe41ecb0b00e7e7f6ff41cd11c`. `git status --porcelain` was empty both times. That tree already contains the Search hash fix (`5984781`). The stdout does not print a SHA.

Command, from `control-plane/`, with `QA_SKIP_TABS` unset:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway` started from that clean tree, fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped.

The commit that adds this log is `6845ede7e55079625e1301b699b52e2701b0b5b1`. It does not change gateway or control-plane code. qa:tabs was not run again for that commit, or for the README line that records its SHA. The stdout does not print either SHA.

[qa-tabs-b4b5-seeded.log](qa-tabs-b4b5-seeded.log) is the same command on the tree before the Search fix. It does not confirm the final code. [qa-tabs-branch-seeded.log](qa-tabs-branch-seeded.log) is the earlier pre-B4/B5 run. Both are kept.

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
| [go-test-before-b4b5.txt](go-test-before-b4b5.txt) | B4 publish tests and B5a/B5b against the pre-fix behavior. The `-run` list names `TestB5a_` and `TestB5b_` explicitly. Memory, SQLite, and Postgres fail |
| [go-test-after-b4b5.txt](go-test-after-b4b5.txt) | Passing run whose `-run` pattern includes `TestB5_`. That pattern selects `TestB5_NamespaceSymlinkRejected` and `TestB5_ParentAndNestedSymlink` only. It does **not** select `TestB5a_` or `TestB5b_`. This file is not a full verbose B5 pass |
| [go-test-b4-b5-full.txt](go-test-b4-b5-full.txt) | Full verbose B4 and B5 pass, including `TestB5a_` and `TestB5b_`, on `5984781feb4e76a857ade2b173d0c03f2da8a186` |
| [go-test-before-b4-search.txt](go-test-before-b4-search.txt) | `TestB4_SearchDoesNotPairOldSnippetWithNewVersion` before the hash check. Memory, SQLite, and Postgres pair snippet V1 with `current` |
| [go-test-after-b4-search.txt](go-test-after-b4-search.txt) | Same test after the hash check, on `5984781feb4e76a857ade2b173d0c03f2da8a186`. All three backends pass |

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

B4 and B5 below were still open on `7a3ee90`. The new tests failed on that behavior, then passed after the change. `go-test-before-b4b5.txt` names `TestB5a_` and `TestB5b_` in its `-run` list. `go-test-after-b4b5.txt` does not: its pattern `TestB5_` never selects those two tests. The full verbose pass is [go-test-b4-b5-full.txt](go-test-b4-b5-full.txt). Each subtest name is the backend.

Before, on memory, SQLite, and Postgres:

- `TestB4_AfterCommitReadersFailClosed`: after the registry commit and before rename, get returned `original-body` with `body_status=current` and the new hash.
- `TestB4_RenameFailureUnconfirmed`: a failed rename still returned the old body as current.
- `TestB4_ReadFileErrorFailClosed`: a read error still returned `cached-body-8841` as current.
- `TestB4_RestartAcrossPersistentStore`: SQLite and Postgres, after close and reopen, still returned the old body as current. Memory has no restart case.
- `TestB5a_PartialSymlinkLeavesOtherTenantUnchanged`: `db-ok` followed the `.partial` symlink; `db-fail` replaced the other tenant's file with `SALES-OVERWRITE-8841`. This is local filesystem access, not a remote exploit.
- `TestB5b_DirSymlinkDoesNotDeleteChild`: sync deleted `nhom/note.md` (`Deleted:1`) after `nhom` was replaced by a symlink.

After, on `5984781feb4e76a857ade2b173d0c03f2da8a186`: those tests PASS on memory, SQLite, and Postgres (restart is SQLite and Postgres only). Command, env name only, from the repo root, with `GOSO_TEST_POSTGRES_DSN` set:

```bash
go test ./gateway/internal/vault/ -count=1 -timeout 180s -run 'TestB4|TestB5' -v
```

The stdout in `go-test-b4-b5-full.txt` does not print a commit SHA. `go test ./gateway/... -count=1` also passed on that tree before the transcripts were copied in.

Get and list return a body and hash only when the file bytes match the registry hash. Search stores that hash with the hit that produced the snippet. The hit is `current` only when the snapshot matches the verified file. If a newer put lands after the query, the hit stays, `body_status` is `unconfirmed`, and the snippet is empty. Search does not build a snippet from the later body. A read error fails closed. Two operating-system processes writing the same vault root are unsupported; the namespace lock is process-local.

### Search race

Fixture: one doc titled `Giữ` for tenant `minh-an-sales`, body `V1-ONLY-8841 shared`, then a put of `V2-ONLY-8841 shared` while search is paused after `SearchVault` and before the refetch. Query: `V1-ONLY-8841`.

Fail transcript [go-test-before-b4-search.txt](go-test-before-b4-search.txt) was captured on `57894583ff6b35885ec99968ac76012e8c69e916` plus the new test and the post-query barrier, before the hash comparison existed. That intermediate tree was not committed. The stdout has no SHA. Memory, SQLite, and Postgres each failed with `Snippet:"V1-ONLY-8841 shared"` and `BodyStatus:"current"`.

Pass transcript [go-test-after-b4-search.txt](go-test-after-b4-search.txt) is the same command on `5984781feb4e76a857ade2b173d0c03f2da8a186`:

```bash
go test ./gateway/internal/vault/ -count=1 -timeout 180s \
  -run 'TestB4_SearchDoesNotPairOldSnippetWithNewVersion' -v
```

Memory, SQLite, and Postgres pass. The hit remains, with an empty snippet and `unconfirmed`. The stdout has no SHA.

## Who may GET the inventory

No new grant. Unauth, the view token, and issued keys scoped only `read` or only `write` are denied. The dry-run is readable by the existing admin bearer (`GOSO_ADMIN_TOKEN`) and by an issued key that already has scope `admin`. An issued key cannot switch tenant with `X-Goso-Tenant`; `TestB2_IssuedKeyCannotSwitchTenant` covers that. The admin bearer can. `TestB2_InventoryDenyLeaksNoMetadata` covers the denies and the two allows.

## Schema

Turning `GOSO_VAULT_SCOPED_SYNC` off does not roll the schema back. Compatibility with the old binary is unproven. There is no automatic rollback.

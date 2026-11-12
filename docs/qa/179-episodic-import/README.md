# SPEC 179 — episodic summaries import

Offline JSONL import of `episodic_summaries` into `memories` (`kind=episodic`). No GoClaw connection, no schema change, no pgvector. The fixture is synthetic Minh An. A denylist is a local file and is not stored in this tree.

`--dry-run` prints, per source tenant: read, write, already, archived sessions to create, and skip counters (`expired`, `customer-private`, `team/scope`, `unmapped`, `missing-relation-endpoint`, `invalid`, `native-exists`). The report also prints `denied_tenants` as a count and does not print denylist names or user ids. A mapped `user_id` keeps that `customer_key`. Otherwise a non-empty `user_id` is `goclaw:<user_id>`. An empty `user_id` is the only empty `customer_key`.

`#44` plan and ceiling gates are not modified. They stay in `gateway/internal/store/postgres_tsvector_test.go`.

## Local run

PostgreSQL 16.15 on localhost. The DSN is not recorded. `CREATE EXTENSION vector` is skipped when the extension is absent; recall still uses `body_tsv`.

```text
go test -race -count=1 -timeout 180s ./gateway/internal/episodicimport ./gateway/cmd/goso-episodic-import
ok  github.com/mqglobal/goso/gateway/internal/episodicimport
ok  github.com/mqglobal/goso/gateway/cmd/goso-episodic-import
```

`TestMinhAnEpisodic` passed on sqlite and postgres. `TestPruneKeepsImportedEpisodic` passed on memory, sqlite, and postgres. `TestDeleteEpisodicGCLeavesOtherPrefix` passed on sqlite and postgres. `go test ./gateway/internal/store/ -run 'TestPostgresRecall|TestRecallORSQL|TestRecallOlder'` passed the plan, index, and row/block ceilings. The 50 ms samples skipped with `GOSO_RECALL_BENCH` unset, after the ceiling check. No shape exceeded `recallPlanCeiling`. `go build ./gateway/...` passed.

## qa:tabs

Code under test: `578984150652242a7ac0134ceb8e194bac1b15c8`.

Log: [qa-tabs-578984150652242a7ac0134ceb8e194bac1b15c8.log](../179/qa-tabs-578984150652242a7ac0134ceb8e194bac1b15c8.log).

Chrome binary `/usr/local/bin/google-chrome`. `QA_SKIP_TABS` and `QA_SHOTS` were unset. Gateway was the binary built from that commit, on `127.0.0.1:8080`, with a fresh SQLite file and vault directory. Seeded agent `minh-an-sales` / `Cố vấn Minh An` / model `echo`. Control plane was `npm run dev` on port 3000.

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Exit 0. Last lines: `flow chat` then `qa:tabs ok`.

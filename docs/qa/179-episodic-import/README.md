# SPEC 179 — episodic summaries import

Offline JSONL import of `episodic_summaries` into `memories` (`kind=episodic`). No GoClaw connection, no schema change, no pgvector. The fixture is synthetic Minh An. A denylist is a local file and is not stored in this tree.

`--dry-run` prints, per source tenant: read, write, already, archived sessions to create, and skip counters (`expired`, `customer-private`, `team/scope`, `mq`, `unmapped`, `missing-relation-endpoint`, `invalid`). A non-empty `user_id` is written as `goclaw:<user_id>`. An empty `user_id` is the only empty `customer_key`.

`#44` plan and ceiling gates are not modified. They stay in `gateway/internal/store/postgres_tsvector_test.go`.

## Local run

PostgreSQL 16.15 on localhost. The DSN is not recorded. `CREATE EXTENSION vector` is skipped when the extension is absent; recall still uses `body_tsv`.

```text
go test -race -count=1 -timeout 180s ./gateway/internal/episodicimport ./gateway/cmd/goso-episodic-import
ok  github.com/mqglobal/goso/gateway/internal/episodicimport
ok  github.com/mqglobal/goso/gateway/cmd/goso-episodic-import
```

`TestMinhAnEpisodic` passed on sqlite and postgres. `go test ./gateway/internal/store/ -run 'TestPostgresRecall|TestRecallORSQL|TestRecallOlder'` passed the plan, index, and row/block ceilings. The 50 ms samples skipped with `GOSO_RECALL_BENCH` unset, after the ceiling check. No shape exceeded `recallPlanCeiling`. `go build ./gateway/...` passed.

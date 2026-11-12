# memory-vi (SPEC 153)

30 Minh An cases and the floor measured on main `9545d08`.

| File | What it is |
|---|---|
| `cases.json` | The 30 cases. Generated from `gateway/memorybench` `Cases()`. The test fails if the two drift. |
| `baseline-9545d08.json` | Pass or miss for every case on SQLite and Postgres at `9545d08`. |

The runner is `go test ./gateway/memorybench` (or `make bench-memory-vi`). It lives under `gateway/` because it calls `store.PutMemory` and `pipeline.RecallFor`, which are in `gateway/internal`. `make verify` runs the same test. CI runs it again with `-v` so the score table is in the log. Postgres runs when `GOSO_TEST_POSTGRES_DSN` is set. CI sets that variable. SQLite always runs.

A case that must return nothing fails the test immediately if any item is admitted. Every other case fails the test only when `baseline-9545d08.json` marks it as a pass and this run marks it as a miss. A higher score does not fail, and does not require editing the floor file.

`GOSO_MEMORY_BENCH_RECORD=1` rewrites the floor. CI refuses that variable.

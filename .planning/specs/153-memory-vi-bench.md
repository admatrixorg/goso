# SPEC 153 — Vietnamese memory recall bench

Status: bench and floor recorded on this branch. One PR into `main`. Do not merge from this spec.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw or Dewee. It does not change memory behavior, the schema, or a control-plane tab.

Linear: https://linear.app/mqglobal/issue/MQG-230

## Number

Checked on `main` `9545d08` before adding this file:

| Number | Where it is used |
|---|---|
| 152 | On `main` (`9545d08`): `.planning/specs/152-vault-import-file-body.md`. |
| 153 | This spec. Not a file on `main` at `9545d08`. |

The Memory v3 document is not on `main` at that commit, so this PR adds only this spec.

## Why

Later memory specs need a number they can compare. At `9545d08` Goso recalls L1 notes by lexical overlap (SQLite FTS5 bm25, Postgres a scan of the newest 500 rows) and admits them with the SPEC 132 rules. There was no checked-in Vietnamese suite, so a change could not show that recall did not drop.

## What this PR adds

`bench/memory-vi/` holds 30 cases (`cases.json`) and the floor (`baseline-9545d08.json`). The runner is `gateway/memorybench`, because `PutMemory` and `RecallFor` live in `gateway/internal` and a package outside `gateway/` cannot import them. The runner is not linked into `goso-gateway`.

For each case the runner wipes the store, writes the case's notes with `store.PutMemory`, and reads with `pipeline.RecallFor`. That is the operator write path and the cross-session prompt path. There is no model call. The current session is excluded. Channel cases set `CustomerScoped` the same way a channel turn does.

A positive case passes when the admitted note contains every gold fragment and none of the forbidden fragments. A zero case passes when `RecallFor` returns no items. Zero cases are `never-said` (the question shares no token with the notes, including diacritics SQLite FTS would fold), `trivial` (`ok cảm ơn`), or `isolated` (the question would match a note the scope filter must hide).

`make verify` runs the package through `go test ./...`. The verify workflow also runs `go test ./gateway/memorybench -v` so the score table is in the CI log. SQLite always runs. Postgres runs when `GOSO_TEST_POSTGRES_DSN` is set, and the test fails in CI when that variable is missing. Each engine is run twice; a different pass map fails the test.

## Floor at 9545d08

Both engines pass 20 of 30 cases. The test fails when a case marked true below becomes false, when a group passes fewer cases, or when any case that must return nothing admits an item. A case marked false that later passes does not fail, and the floor file does not need an edit.

| Group | Cases | SQLite | Postgres |
|---|---:|---:|---:|
| Trích xuất thông tin | 6 | 6/6 | 6/6 |
| Suy luận nhiều phiên | 4 | 3/4 | 4/4 |
| Cập nhật tri thức | 4 | 0/4 | 0/4 |
| Thẩm quyền | 4 | 0/4 | 0/4 |
| Từ chối | 4 | 4/4 | 4/4 |
| Cô lập | 4 | 4/4 | 4/4 |
| Tiếng Việt không dấu, đồng nghĩa | 4 | 3/4 | 2/4 |
| Total | 30 | 20/30 | 20/30 |

Misses at this floor, same on a second run:

| Case | SQLite | Postgres | What the run did |
|---|---|---|---|
| sl-03 | miss | pass | Three notes. SQLite bm25 plus the 0.5× rule admitted one. Postgres token overlap admitted all three. |
| cn-01 .. cn-04 | miss | miss | The old value and the new value were both admitted. |
| tq-01 .. tq-04 | miss | miss | The user note and the extractor note were both admitted. There is no authority rank on `memories`. |
| tv-03 `gui bao gia` | pass | miss | SQLite FTS5 folded diacritics and found `báo giá`. Postgres substring match did not. |
| tv-04 `quotation sheet` | miss | miss | The English words are not in the note, so nothing was admitted. |

Từ chối is 4/4 on both engines. tc-01, tc-03, and tc-04 ask about something the notes never said and admit nothing. tc-02 is the trivial acknowledgement `ok cảm ơn` and also admits nothing. cl-01, cl-02, and cl-03 admit nothing across customer, tenant, and agent. cl-04 admits the asking customer's note and not the other customer's note.

## Tests

`TestValidateMinhAn` checks the 30-case shape, the Minh An sample rule (no email, no 8-digit run, no banned customer strings), and that every never-said query is disjoint from its notes even after diacritic folding.

`TestBaseline9545d08` runs both engines and compares to `bench/memory-vi/baseline-9545d08.json`.

## Non-goals

No change to recall, admission, FTS, or Postgres search. No new memory column. No model answer. No 200-case set. No control-plane change, so prototype screenshots do not apply.

## Rollback

Delete `bench/memory-vi/`, `gateway/memorybench/`, this file, `docs/qa/153-memory-vi-bench/`, the SPEC 153 changelog line, and the memory-vi step in `.github/workflows/ci.yml`. No schema change and no flag to turn.

# SPEC 153 — Vietnamese memory bench

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only: household-goods distribution, teams `kinh-doanh` and `phap-che`, agents Cố vấn pháp lý, Cố vấn chiến lược, Cố vấn tài chính, and Trợ lý giám đốc, project `mien-tay` (“Mở kênh đại lý miền Tây”), contacts `dai-ly-mau` and `dai-ly-mien-tay`. The company discount in the fixtures is 10%. The `mien-tay` discount is 9%. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted: the Dewee memory article, the public GoClaw memory and vault articles, and the memory articles already cited by the 12/11 plan. This spec does not copy either codebase.

## Number

Checked on `main` at `9545d08` before assigning these numbers. No file under `.planning/specs/` uses 153, 154, 155, 156, 157, 158, 159, 160, 161, 162, 163, 164, 165, 166, 167, 168, or 169.

| Fact | Meaning |
|---|---|
| 136 | Held by open PR #10 (Project in Trò chuyện). Not a number in this plan. |
| 140 | Held by open PR #24 (`.planning/specs/140-jev-knowledge-filter.md`). Not a number in this plan. |
| 134 and 137 | Old queue rows in `.planning/specs/goso-2026-10-31-queue.md`. Their ideas move to SPEC 160 (the approval queue that was 137), SPEC 161 (facts and reconcile), and SPEC 164 (scope, ACL, and forget). Do not open a new SPEC 134 or SPEC 137. |
| 141–146 | Draft numbers from the 12/11 Memory v3 note. This plan does not use them. |
| 153–167 | This plan, one spec per later code PR. Numbering starts at 153 because 136 and 140 are taken. |
| 168 and 169 | Pending Dat’s approval only. Those files are not designs. |

## Why

Memory changes need a score that cannot go down, on both databases Goso already ships. `main` at `9545d08` has no `bench/memory-vi/` directory. This spec adds that bench and records the baseline before SPEC 154–167 change recall.

## Behavior

1. Add `bench/memory-vi/` with 30 cases. Every case is Minh An. A case is a short session fixture, one question, the expected answer or the expected empty result, and one group: `trich-xuat`, `cap-nhat`, `tham-quyen`, `tu-choi`, `co-lap`, or `tieng-viet`.
2. CI runs the same 30 cases on SQLite and on Postgres (`GOSO_TEST_POSTGRES_DSN`, the `postgres:16` service already in `.github/workflows/ci.yml`).
3. The run writes the baseline for commit `9545d08`: per-group score, SQLite and Postgres. Later specs compare to that file. They do not invent a second baseline.
4. Token totals in the report use the single counting function in SPEC 165. This bench does not define a token cap.
5. The writer of the bench is the implementation of this spec. The cases stay Minh An.

One fixture question, and the only people and companies in the set: “Chiết khấu dự án miền Tây của Minh An là bao nhiêu?” expects `9%` when the fixture stored that project fact. A `tu-choi` question asks for a warehouse city the fixtures never mention and expects no hit.

## Groups later specs gate on

| Group | What a later spec requires |
|---|---|
| `trich-xuat` | SPEC 156: the score does not fall below this baseline. |
| `cap-nhat` | SPEC 162: at least 90% once that spec is on. |
| `tham-quyen` | SPEC 161: 100% before any fact leaves shadow. |
| `tu-choi` | A question the fixtures never stated returns no hit. |
| `co-lap` | Another tenant, and the other Minh An contact, return no hit. |
| `tieng-viet` | SPEC 166: at least 85% once that spec is on. Includes the query `bao gia` against a note that says `báo giá`. |

## Decisions already closed

- Facts stay in shadow for 14 days. They become active only when `tham-quyen` is 100% and Dat approves the move.
- Admission tier 2 stays off (SPEC 165).
- Every new memory flag stays off in production until Dat approves that flag.
- SPEC 168, SPEC 169, moving held personal vault documents to `scope=user`, and installing `unaccent` or `pgvector` for a Postgres tenant are separate approvals. They are not part of this bench.

## Gate

- **Invariant.** The bench does not use real data.
- **Acceptance.** The run records the baseline score of `9545d08` on SQLite and on Postgres.
- **Negative test.** A question about something the fixtures never said returns no hit.
- **Rollback.** Delete the `bench/memory-vi/` directory.

## Non-goals

A 200-case set, a public leaderboard, and any product behavior change. SPEC 154–167 do not land in this PR.

# SPEC 156 — Absolute recall threshold

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only, the same fixtures as SPEC 153. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 156. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

Recall already drops a greeting or an acknowledgement, and it already keeps a hit only when the score is at least half of the best hit (`recallRelative` in `gateway/internal/pipeline/recall.go`). A weak relative hit can still enter the prompt. This spec adds an absolute floor. The floor is a measurement from SPEC 153, not a number chosen in this file.

## Behavior

1. Add `GOSO_MEMORY_RECALL_MIN_SCORE`. The implementation PR sets the default from the SPEC 153 baseline so that `trich-xuat` does not fall. This spec does not hard-code the default.
2. A hit must pass both gates: the existing relative gate, and a score at or above the absolute floor.
3. The queries `ok` and `cảm ơn` return no item.
4. A meaningful Minh An question that has a matching note, such as the `mien-tay` discount, still returns that note.
5. Re-run SPEC 153. The `trich-xuat` score is greater than or equal to the `9545d08` baseline.

## Gate

- **Invariant.** The `trich-xuat` group does not go down.
- **Acceptance.** The queries `ok` and `cảm ơn` return no item.
- **Negative test.** A meaningful question is not dropped.
- **Rollback.** Set `GOSO_MEMORY_RECALL_MIN_SCORE` to `0`. That disables only the absolute floor. The relative gate stays.

## Non-goals

A second token budget. The only token budget in this plan is the table in SPEC 165. Changing prompt order (SPEC 157).

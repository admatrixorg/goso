# SPEC 165 — Admission, tier 1

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only, the SPEC 153 fixtures. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 165. Loaded rows already carry `scope` and `owner_id` when they come from `memory_facts` (SPEC 161) or `profiles` (SPEC 162). Admission does not invent a row that lacks them, and it does not drop those columns.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140 (Jev knowledge filter, branch `cursor/jev-knowledge-filter-f553`). PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164.

If PR #24 is on `main` before this implementation starts, extend that filter. Do not add a second scorer beside it. If PR #24 is not on `main`, tier 1 below is the whole scorer. Do not copy a filter from another repository while waiting.

## Token budget

One table. One counting function, `CountMemoryTokens`. Every number in the table is an integer returned by that function for the text actually inserted into the single system message (SPEC 157). No other memory budget is defined in SPEC 153–169. Do not add a rune cap, a second token cap, or a per-spec cap beside this table.

| Row | Cap, counted by `CountMemoryTokens` |
|---|---|
| Profile, static | 250 |
| Profile, dynamic | 150 |
| Loaded items | 500 |
| Total | 900 |

The total is the static count plus the dynamic count plus the loaded-items count. It is at most 900. An empty region counts as 0. SPEC 162 fills the two profile rows and does not set its own numbers. This spec fills the loaded-items row. While `GOSO_MEMORY_ADMISSION` is off, injection stays as it is on `9545d08`, and this table is not applied on top of that path. While the flag is on, this table is the only cap. The previous recall cap is not also applied.

## Behavior

1. Flag `GOSO_MEMORY_ADMISSION`. Default off.
2. On, skip a trivial turn with the existing greeting and acknowledgement check. For every other turn:
   1. Candidates are the dynamic profile (it spends the profile rows, not the loaded-items row), at most 5 recall notes, at most 8 facts, and at most 8 vault or knowledge-graph hits. The vault and knowledge-graph candidates exist only when SPEC 166 is on. The 5 and the 8 are candidate counts, not token caps.
   2. Drop a duplicate slot (`subject` + `predicate`), keeping the newest. Drop a near-duplicate body, keeping the higher score.
   3. Tier 1 scores without a model: shared entity with the question, same contact, and a `failed` outcome (SPEC 163) with the same tool signature.
   4. Tier 2 is `GOSO_MEMORY_ADMISSION_LLM`. Default off, including for every current agent. It is one cheap-model call that scores each candidate from 0 to 3, with a timeout of 1.5 seconds. A timeout or an error returns the tier-1 result.
   5. Fill the loaded-items row of the table above. Drop whole candidates past the cap. Do not cut a candidate in the middle.
3. ACL never fails open. A candidate the caller cannot read (SPEC 164, and tenant isolation before that) is not inserted when a scorer times out, errors, or is off. Fail-open applies only to the usefulness score.
4. `admission_log` stores the session id, candidate ids, admitted ids, the top score, and the latency. It does not store message content.
5. Acceptance is measured by SPEC 153 using `CountMemoryTokens`.
6. Production stays off until Dat approves this flag. Tier 2 stays off until a separate approval.

## Gate

- **Invariant.** ACL never fails open.
- **Acceptance.** Tokens per turn fall by at least 20%. Recall falls by at most 2 points on the SPEC 153 bench.
- **Negative test.** A tier-2 timeout still returns the tier-1 result.
- **Rollback.** Turn `GOSO_MEMORY_ADMISSION` off. Turn `GOSO_MEMORY_ADMISSION_LLM` off with it.

## Non-goals

Choosing a model for tier 2. Enabling tier 2 by default. A second budget table.

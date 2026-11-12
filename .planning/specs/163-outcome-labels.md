# SPEC 163 — Outcome labels

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. A failed note on a `mien-tay` quote says `thiếu VAT`. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 163. Outcome columns are new on episodic `memories`. They are not a substitute for `scope` and `owner_id`. Those columns already exist on `memory_facts` (SPEC 161). SPEC 164 adds them to `memories`. This spec does not.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164.

## Why

An episodic note does not record whether the turn worked. Recall then cannot prefer a known failure, and SPEC 167 cannot see a repeated success. The chat control that records a person’s label is a visible tab change, so the implementation PR includes side-by-side screenshots of the prototype and of Goso for every tab it changes.

## Behavior

1. Flag `GOSO_MEMORY_OUTCOME`. Default off. Off: no control, no automatic label, and recall scoring is unchanged.
2. On, additive columns on `memories` where `kind=episodic`: `outcome` (`success`, `partial`, `failed`, `unknown`), `outcome_note` (one short line), `outcome_by` (`user`, `operator`, `auto`), `outcome_at`.
3. The person uses a 👍 or 👎 control at the end of the turn. 👎 asks for that one line. The operator can set the same fields from the Memory tab.
4. Automatic labels are heuristics. They do not call a model. The same request again within 10 minutes sets `partial`. Two tool errors set `failed`. One tool error does not set `failed`. A session that ends after the agent delivered a file, and that file was not corrected, sets `success`.
5. Logs store the memory id and the outcome enum. They do not store the note text, the user text, or the assistant text.
6. Recall uses the label. A `failed` note is preferred when the tool signature matches the current turn, so the miss is visible. A `success` note is what SPEC 167 groups. The preference does not bypass tenant, agent, or contact isolation.
7. The note length is a field limit. It is not a prompt budget. The only prompt budget is the table in SPEC 165.
8. Production stays off until Dat approves this flag.

## Gate

- **Invariant.** Logs do not store the content.
- **Acceptance.** A label changes the recall score, as in step 6.
- **Negative test.** A single tool error does not set `failed`.
- **Rollback.** Turn `GOSO_MEMORY_OUTCOME` off.

## Non-goals

Writing a skill (SPEC 167, and only through SPEC 160). A second system message.

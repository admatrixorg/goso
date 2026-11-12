# SPEC 161 — memory_facts and shadow reconcile

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. The sample slot is subject `minh-an`, predicate `chiet-khau`, qualifier `toan-cong-ty`, value `10%`. The project qualifier `mien-tay` has value `9%`. No MQ Capital. No customer data. No real names. No phone numbers. Tests that must trip the PII and secret filters use a generated pattern. They do not embed a real number, a real address, or a live credential.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. Reconcile is a deterministic rule in this repo. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 161.

`scope` and `owner_id` are created here. SPEC 162, 163, 164, 165, 166, and 167 assume the columns already exist. SPEC 164 turns on read rules. It does not add these columns to `memory_facts`.

Old SPEC 134’s “a lower-authority write cannot erase a higher one” and old SPEC 137’s queue are this spec plus SPEC 160 and SPEC 164. Do not implement 134 or 137 beside them.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136.

## Why

`memories` stores episodic text and operator durable notes. It has no subject, predicate, authority, or validity. A later sentence can only sit beside an older one. This spec adds `memory_facts` as an additive table. It does not alter `memories`.

## Schema

New table only. `scope` and `owner_id` are in the first migration, not a later one.

| Column | Meaning |
|---|---|
| `id`, `tenant_id`, `agent_id` | Identity. `agent_id` may be null. |
| `scope` | One of `contact`, `user`, `project`, `team`, `agent`, `tenant`. Default `agent`. |
| `owner_id` | Default: the tenant id. For `scope=contact`, the contact id. |
| `user_key`, `contact_id`, `project_id` | Nullable. `user_key` is the SPEC 158 principal, and only when that flag is on. It is never the raw header. |
| `subject`, `predicate`, `qualifier`, `value` | The slot. Sample: `minh-an` / `chiet-khau` / `toan-cong-ty` / `10%`. |
| `cardinality` | `one` or `many`. |
| `valid_from`, `valid_until`, `expires_at`, `observed_at` | Validity is not the time the line was heard. |
| `status` | `active`, `superseded`, `disputed`, or `deleted`. `status=active` means “current row of the slot”, not “loaded into the prompt”. |
| `verification` | `explicit` or `inferred`. |
| `asserted_by` | `operator`, `user`, `agent`, or `extractor`. Rank is operator, then user, then agent, then extractor. |
| `confidence` | 0 to 1. Used only to break a tie at the same rank. |
| `source_refs` | JSON list of `{kind, id}` where `kind` is `session`, `vault`, `channel`, or `import`. |
| `supersedes_id`, `version` | History for an `as_of` read. |
| `mode` | `shadow` or `active`. Every new row is `shadow`. |

Partial unique index, from this migration onward:

`(tenant_id, scope, owner_id, subject, predicate, qualifier) WHERE status='active' AND cardinality='one'`

A channel source cannot land in any scope except `contact`. Reconcile returns `skip` if the writer asks for `tenant` (or any other scope) on a channel source. Knowledge-graph extraction stays off for channel transcripts, as it is today.

## Reconcile

Pure rules. No model call.

1. `skip` when the row has no source, no authority, an impossible time, a secret-shaped value (token, key, or card pattern), a phone-shaped value, or an email-shaped value.
2. Same slot and same value: `extend`. Append `source_refs`, refresh `observed_at`, do not bump `version`.
3. No current value, or `cardinality=many`: `create`.
4. Different value, `valid_from` newer than the current row, and authority greater than or equal to the current row: `supersede`. At the same rank, `explicit` beats `inferred`. The old row becomes `superseded` and its `valid_until` is the new `valid_from`. The new row’s version is the old version plus one.
5. Anything else: `dispute`. Both rows are `disputed` and a SPEC 160 queue item is written. While `mode=shadow`, the prompt still receives nothing from this table.

The Minh An check: a user, explicit, sets `chiet-khau` / `toan-cong-ty` to `10%`. An extractor, inferred, later asserts `8%`. The result is `dispute`. The stored user value remains `10%`. The extractor does not win.

`mien-tay` / `9%` is a different qualifier. It does not collide with `toan-cong-ty`.

## Shadow

Flag `GOSO_MEMORY_FACTS`. Default off. Off: reconcile does not run. On: writes happen, and `mode` stays `shadow`. Shadow rows are stored, not loaded, and not returned by search.

A row stays in shadow for at least 14 days. Moving a row to `mode=active` requires both of the following, and neither is enough alone:

- SPEC 153 group `tham-quyen` scores 100%.
- Dat approves the move, and SPEC 160 has approved that row.

This spec does not perform that move.

## Gate

- **Invariant.** A fact taken from a channel stays at `scope=contact`. Phone-shaped values, email-shaped values, and secret-shaped values are rejected.
- **Acceptance.** The SPEC 153 `tham-quyen` group scores 100% with the flag on (still in shadow).
- **Negative test.** An extractor write does not overwrite a user write. The `10%` user fact is still `10%` after the extractor asserts `8%`.
- **Rollback.** Turn `GOSO_MEMORY_FACTS` off. Shadow rows stay in the table and stay unloaded. The `scope` and `owner_id` columns stay.

## Non-goals

Read ACL (SPEC 164). Profiles (SPEC 162). Changing columns on `memories`. Loading shadow rows into the prompt.

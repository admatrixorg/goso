# SPEC 162 — Dynamic profile

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. The contact profile for `dai-ly-mau` may say that Minh An distribution confirmed a company discount of 10% and that project `mien-tay` is 9%, and that a quote is a PDF that includes VAT. No MQ Capital. No customer data. No real names. No phone numbers. No salary, health, or account figure appears in either profile body.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 162. `memory_facts.scope` and `memory_facts.owner_id` already exist (SPEC 161). This table carries the same two columns from its first migration.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164.

## Why

Facts are invisible while they are in shadow, and recall is a search. A channel turn needs a short profile of the contact it is already talking to. User profiles need a real principal, so they wait for SPEC 158. Contact profiles do not.

## Behavior

1. Flag `GOSO_MEMORY_PROFILE`. Default off. Off: no profile is built and the SPEC 157 profile slots stay empty.
2. On, contact first. Build a profile for `(tenant_id, scope=contact, owner_id=contact id)`. A channel turn loads only the profile of the contact on that turn. `prompt_mode=none` loads none.
3. A user profile (`scope=user`, `owner_id` = the SPEC 158 principal) is built only when `GOSO_MEMORY_PRINCIPAL` is on. While that flag is off, this spec builds no user profile and loads none.
4. Columns from the first migration: `tenant_id`, `scope`, `owner_id`, `contact_id` (null for a user profile), `static_body`, `dynamic_body`, `static_hash`, `built_at`. SPEC 164’s `forget(contact)` deletes these rows. This spec does not add a second scope column later.
5. Static body: durable explicit facts (role of the agent, the confirmed discount, the quote format). Rebuilt when a fact is created or superseded, and at most once an hour. `static_hash` stays unchanged through a day when the static facts did not change, so SPEC 157’s prefix hash stays stable.
6. Dynamic body: a few open items and what changed in the last 7 days, from episodic notes and recently observed facts. Rebuilt when a session ends, together with the existing idle flush.
7. The rebuild asks a cheap model for bullet lines, not prose. The bullets are still only Minh An demo content in tests.
8. The sensitive predicate list is a fixed constant: `suc-khoe`, `luong`, `so-tai-khoan`. A fact with one of those predicates is never copied into either body. The test list and the constant are the same list. Adding a predicate is a change to this spec.
9. The static cap and the dynamic cap are the profile rows of the single token table in SPEC 165, counted by the function named there. This spec does not define another cap. Text that does not fit is dropped by score, not cut mid-bullet.
10. Production stays off until Dat approves this flag.

The isolation check uses the two Minh An contacts. `dai-ly-mau` does not see the profile of `dai-ly-mien-tay`.

## Gate

- **Invariant.** The sensitive predicate list is the fixed constant above.
- **Acceptance.** With the flag on, the SPEC 153 `cap-nhat` group scores at least 90%.
- **Negative test.** Contact `dai-ly-mau` does not see the profile of contact `dai-ly-mien-tay`.
- **Rollback.** Turn `GOSO_MEMORY_PROFILE` off. Rows may remain. They are not loaded.

## Non-goals

A user profile before SPEC 158. Admission ranking (SPEC 165). Storing a phone or an email in the profile.

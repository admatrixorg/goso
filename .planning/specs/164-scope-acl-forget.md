# SPEC 164 — Scope, ACL, and forget

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. Project `mien-tay` holds the 9% discount. A viewer in tenant `minh-an` must not read it. Contacts `dai-ly-mau` and `dai-ly-mien-tay` stay isolated. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 164.

`memory_facts.scope` and `memory_facts.owner_id` exist since SPEC 161, including the partial unique index that contains them. This spec does not add those columns to `memory_facts` and does not rebuild that index. `profiles` already have `scope` and `owner_id` (SPEC 162). `context_files` already have them (SPEC 150). This spec does not replace that table.

Old SPEC 134’s project scope, and the forget half of the old memory plan, are this spec. Old SPEC 137’s queue is SPEC 160. Do not implement 134 or 137 beside them.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. PR #10 is the project UI. This spec is the memory ACL that a project-scoped fact needs. It does not take PR #10’s number.

## Why

Isolation today is tenant, agent, and `customer_key` on a channel. There is no rule that a viewer cannot read a project fact, and deleting a contact does not delete the facts derived from that contact. SPEC 158 is the principal these rules use. A member label that was not authenticated is not a principal.

## Behavior

1. Flag `GOSO_MEMORY_ACL`. Default off. Off: reads stay on the `9545d08` rules (tenant, agent, customer key). `scope` and `owner_id` are still stored. They are not consulted. `forget` is refused while the flag is off.
2. Add `scope` and `owner_id` to `memories` and to `vault_docs`, additive, default `agent` and the tenant id. Do not change the meaning of `owner_explicit`. Do not drop it in this spec. Leave `memory_facts`, `profiles`, and `context_files` on the columns they already have.
3. The principal is SPEC 158. While `GOSO_MEMORY_PRINCIPAL` is off, this flag cannot open a wider read. Missing principal means return nothing (fail-closed), not the whole tenant.
4. With both flags on, a `viewer` may read `tenant` and `agent` only. A `member` may also read `project` and `team` rows they belong to. `owner` and `admin` may read all staff scopes. A `viewer` who requests project `mien-tay` gets no row.
5. Staff precedence for the same slot is `user`, then `project`, then `team`, then `agent`, then `tenant`. The narrowest row is loaded. A wider row is loaded only when it does not contradict the narrower one. The sample pair is company `10%` at tenant scope and `mien-tay` `9%` at project scope: inside that project the answer is `9%`.
6. `scope=contact` is not a rung on that ladder. It is readable only on a turn bound to that contact (SPEC 161). It is never widened to `tenant`.
7. `soft_delete` sets `status=deleted`. The row is not loaded and not searched. It is kept for 30 days for the audit, then removed by the existing cleanup cycle.
8. `forget(source)` deletes that source and every fact whose `source_refs` point at it. A derived knowledge-graph node is closed with `valid_until` set to now.
9. `forget(contact)` deletes that contact’s episodic notes, facts, profile rows, derived knowledge-graph nodes, and vector rows from SPEC 166 (none, until 166 has shipped). It requires `confirm`. It writes an audit row. Only `owner` or `admin` may call it. Forgetting `dai-ly-mau` leaves `dai-ly-mien-tay` in place.
10. Held personal vault documents move to `scope=user` only after Dat approves that move, one tenant at a time. This spec does not run that move. The count Dat was given for that decision is 1,146 documents.
11. Production stays off until Dat approves this flag.

## Gate

- **Invariant.** ACL uses the SPEC 158 principal.
- **Acceptance.** `forget(contact)` deletes the derived data for that contact.
- **Negative test.** A viewer does not read `scope=project`.
- **Rollback.** Turn `GOSO_MEMORY_ACL` off. Columns stay. `forget` does not run. A `forget` that already committed is not reversed; its audit row stays. `forget` itself requires `confirm` and an audit row.

## Non-goals

A full RBAC matrix beyond viewer, member, owner, and admin. Designing SPEC 168 or SPEC 169. Installing pgvector.

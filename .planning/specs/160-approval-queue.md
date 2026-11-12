# SPEC 160 — Knowledge approval queue

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. A queued row is the company discount fact from SPEC 161, or a `bao-gia` skill proposal from SPEC 167. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 160.

Old SPEC 137 in `.planning/specs/goso-2026-10-31-queue.md` was “one Phê duyệt queue”. That queue item is this spec. Do not also implement 137. The memory-backend ideas in old SPEC 134 are SPEC 161 and SPEC 164, not a third queue.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136.

## Why

SPEC 161 writes facts in shadow, and SPEC 167 only proposes skills. Something has to accept or refuse the promotion. Nothing that is still a proposal may be treated as active knowledge. The approvals page already exists for tool approvals. This spec adds the knowledge-promotion kind on that queue. It does not rebuild shell approvals.

## Behavior

1. Flag `GOSO_MEMORY_APPROVAL`. Default off. Off: the new routes return 404, and no shadow fact and no skill proposal is promoted.
2. On: a knowledge row is `pending` until an `owner` or `admin` approves or rejects it. Approve and reject each write an audit row: actor principal (SPEC 158 when that flag is on, otherwise the admin credential id), time, decision, and the target id. The audit row does not store the message text.
3. A `viewer` cannot approve or reject. The call returns 403 and writes nothing.
4. Approve is the only transition that marks a fact `mode=active` or that writes a skill. Reject leaves the fact in shadow, or drops the skill proposal, and records the audit row.
5. Existing tool and shell approvals keep their current behavior.
6. Production stays off until Dat approves this flag.

## Gate

- **Invariant.** Nothing enters active before it is approved.
- **Acceptance.** Approve and reject both write an audit row.
- **Negative test.** A viewer cannot approve.
- **Rollback.** Turn `GOSO_MEMORY_APPROVAL` off.

## Non-goals

Meetings, Decision R2/R3, and a new shell-approval model. Those stayed in the old 137 wording and are not this spec.

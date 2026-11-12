# SPEC 158 — Authenticated user_key

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only: two API keys in the `minh-an` tenant, called key A and key B in tests. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 158. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

`user_key` is taken from the `X-Goso-User` header or from JSON. The client chooses it. `tenant_registry_members.subject` is a label stored by SPEC 151. It is not tied to a token. A caller can claim another person’s files. SPEC 162 user profiles and SPEC 164 ACL need a principal that the client cannot invent.

## Behavior

1. Flag `GOSO_MEMORY_PRINCIPAL`. Default off. Off is fail-closed for anything that would be per user: no user-scoped read, no user-scoped write, and the header is ignored. Tenant, agent, and customer recall stay as they are on `9545d08`. Off does not mean “trust the header again”.
2. On: `user_key` is the principal of the credential that authenticated the request (an API key, or a login session). `tenant_registry_members.subject` matches a memory row only when it equals that principal. The label alone is not a principal.
3. `X-Goso-User`, and a `user_key` field in JSON, are ignored unless the credential is a trusted bridge key that is allowed to forward a subject. Any other header is not an identity.
4. A request that asks for user scope with no principal returns nothing. It does not fall back to another user’s rows.
5. Key A cannot read a user-scoped row written under key B. The tests use Minh An keys only.
6. Production stays off until Dat approves this flag.

## Gate

- **Invariant.** Missing identity returns nothing (fail-closed).
- **Acceptance.** Key A cannot read user B’s data.
- **Negative test.** A forged header, or a missing scope, returns 403.
- **Rollback.** Turn `GOSO_MEMORY_PRINCIPAL` off. User-scoped memory stays unread. Existing tenant, agent, and customer recall stay.

## Non-goals

Durable contacts (SPEC 168, not designed). Replacing `customer_key` on channel notes (SPEC 169, not designed). SPEC 150 context files keep their current flag until this flag is on; while this flag is off they do not gain a new identity path.

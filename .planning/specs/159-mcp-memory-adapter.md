# SPEC 159 — MCP memory adapter

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. A fixture note is the `mien-tay` discount. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted, including the public GoClaw memory article. The compatibility tool name below is a name. It is not a copy of that product’s code. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 159. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

SPEC 052 already serves `/v1/memory` with the same handler as `/api/memory`. That handler speaks `session_id`, `kind`, and `body`. An MCP memory client speaks `path` and `content`. Pointing the client at the alias does not translate the body. This spec adds a translation layer. It does not change `/api/memory`.

## Behavior

1. Add an adapter the MCP client can call for list and get. The adapter reads and writes through the existing memory store. `/api/memory` request and response fields stay `session_id`, `kind`, and `body`.
2. The adapter maps `path` and `content` onto that store. A list returns paths inside the caller’s tenant. A get returns the body of one path in that tenant.
3. The tool may be registered under the name `goclaw_memory_list` so an existing client can call it. The handler is written in this repo. It does not import or paste GoClaw source.
4. A path outside the caller’s tenant returns 404. The body of that 404 does not include another tenant’s note.
5. The SPEC 052 alias stays a same-handler alias. This spec does not give it a second implementation.

## Gate

- **Invariant.** `/api/memory` does not change.
- **Acceptance.** The MCP list tool and the MCP get tool both succeed for a Minh An note in the caller’s tenant.
- **Negative test.** A path outside the tenant returns 404.
- **Rollback.** Remove the adapter route. `/api/memory` and the SPEC 052 alias stay.

## Non-goals

A new memory schema, a copy of a GoClaw module, and a change to context-file routes.

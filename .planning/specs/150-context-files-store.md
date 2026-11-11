# SPEC 150 — Context files store

Status: implemented on `cursor/context-files-store-8978`.
Gap: goso#28 item 2 (agent and user context files). One spec item. No importer.

Sample data is Minh An only. This spec does not copy GoClaw and does not read customer data.

## Why a new table

Checked on `main` (`ff153e5`) before adding anything:

| Existing store | Why it does not hold these files |
|---|---|
| `agents.instructions` | One string already injected as the operator prompt. A set of named files, plus a second set per user, does not fit that column. Writing them there would change the prompt that exists today. |
| `GOSO_CONTEXT_DIR` (SPEC 051) | One process directory (`SOUL.md`, `IDENTITY.md`, `AGENTS.md`, optional `USER.md`). Not per tenant, not per agent, not per user, and not a row a later importer can address. |
| `vault_docs` (PR #25) | Tenant knowledge with `(tenant_id, path)` uniqueness, file sync, and search. Paths are not agent or user grants. There is no user id. Instruction text would show up as knowledge. This spec does not touch the vault importer. |
| `memories` | Goso recall, customer scope, and idle flush stay as they are. These files are not episodic notes. Nothing here writes or reads `memories` differently. |
| `internal/skill` | One shared `GOSO_SKILLS_DIR` of `SKILL.md` folders. Not per tenant or per user. Skills are procedures, not persona or notes. |

One additive table, `context_files`, covers both scopes. It is not two GoClaw-shaped tables.

## Behavior

1. Flag `GOSO_CONTEXT_FILES`. Default off (`1` / `true` / `yes` / `on` enable it). Off: `PUT`/`GET`/`DELETE /api/context-files` return 404 and do not call the store. Chat does not read the table. `agents.instructions`, memory, vault, and `GOSO_CONTEXT_DIR` behave as they do today.
2. On: a file is `(tenant_id, scope, owner, name)`. `scope=agent` owner is the agent id, and the agent must belong to that tenant. `scope=user` owner is an opaque `user_key` (letters, digits, `.`, `_`, `-`; 1–64). The same user key in two tenants is two files. Reads and deletes always filter tenant and owner. A body `tenant_id` is ignored; the tenant is `tenant.Resolve`.
3. Names are a single path segment, 1–64, no `..` and no slash. One body is capped at 32KiB. One owner is capped at 16 files and 64KiB total. A write past either owner limit returns HTTP 400 `too_large` and leaves the previous rows unchanged. Nothing is truncated.
4. When the flag is on, API chat appends the turn agent's files, then the turn user's files, after bootstrap, in the one system message. `prompt_mode=none` does not read them. A set that is already over the owner cap (legacy rows) fails the turn with `too_large` before the model runs. A store error fails the turn (HTTP 500, body `store`) before the model is called and before the user message is saved. The log line is `goso context-files: op=<operation> class=<class>` and does not include the error string, the body, or the user key. The view token cannot PUT or DELETE these routes.
5. `X-Goso-User` or chat JSON `user_key` selects the user file. Both empty means agent files only. The header is ignored while the flag is off. Channel prompt code is unchanged.

## Non-goals

The GoClaw importer, channel adapters, MCP, the vault importer, contacts, memory changes, and a control-plane tab.

## Tests

Store isolation on memory, SQLite, and Postgres (`GOSO_TEST_POSTGRES_DSN` in CI). Pipeline: flag off does not read; flag on, the sample agent answers from the Minh An file; another user and another tenant do not see it; store failure returns. HTTP: forged tenant, cross-tenant GET, 500 body is `store`.

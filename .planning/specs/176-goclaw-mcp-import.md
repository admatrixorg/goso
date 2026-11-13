# SPEC 176 — GoClaw MCP and tool-config import

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw. It does not add a table, a column, or a secret store. There is no pgvector change.

## Number

| Number | Where it is used |
|---|---|
| 172 | Open PR #46: offline contacts import. The command shape follows that importer. |
| 173 | Open PR #45: offline context-file import. The command shape follows that importer. |
| 176 | This spec. Not a file on `main` at `b1bcfa6`. |

## Source

The input is one export file. It is not a live database. JSON is one object whose keys are the GoClaw table names. JSONL is one object per line with a `table` field and the same column names. A file over 32MiB is refused before decode.

Tables this importer reads:

`mcp_servers`, `mcp_agent_grants`, `mcp_user_grants`, `builtin_tools`, `builtin_tool_tenant_configs`.

`mcp_user_credentials` and `mcp_context_credentials` are recognized only so they can be counted and dropped. Their secret fields are not kept after decode. Any other top-level JSON key is ignored and is not stored. A JSONL line whose `table` is not one of those seven names fails the read, so a typo is not dropped silently.

## Where goso keeps MCP and tools

Read from the store on this branch. No new table.

| goso | What it holds |
|---|---|
| `connectors` | One row per connector. The primary key is `name`. There is no `tenant_id` and no source-id column. Fields used here: `name`, `transport`, `endpoint`, `credential_ref`, `schema_version`, `enabled`, `timeout_ms`. |
| `agent_connectors` | `(agent_id, connector_name)`. No enabled flag and no tool allow list. |
| `tool_flags` | Global builtin on/off. Not per tenant. |
| `agent_tool_flags` | Per agent, not per tenant. |
| `secrets` | Ciphertext. This importer does not write it. |
| `builtin` catalog | Compiled tool names. Not a table of definitions. |

The connector name this importer inserts is `goclaw-<tenant>-<slug(name)>`. The source-id map is the existing `schema_version` text: `goclaw-src:<dest tenant>:<source id>`. A later run that sees that exact value skips the row and does not change the stored connector.

## Column mapping

| Source | goso | Rule |
|---|---|---|
| `mcp_servers.id` | `connectors.schema_version` | `goclaw-src:<dest tenant>:<id>`. This is the idempotency key. |
| `mcp_servers.tenant_id` | name prefix and the marker | `--tenant` is the goso tenant written into the name and the marker. `--source-tenant`, when set, keeps only that export tenant. Any other tenant is `foreign-tenant` and is not written. |
| `mcp_servers.name` | `connectors.name` | Slug of the goso tenant plus the slug of `name`. |
| `mcp_servers.transport` | `connectors.transport` | `stdio` → `mcp-stdio`. `sse`, `http`, `mcp`, `streamable-http` → `mcp-http`. Anything else is `invalid-transport`. |
| `mcp_servers.command` and safe `args` | `connectors.endpoint` | Stdio command and arguments, joined verbatim. The command string is scanned too. A credential in either is not copied. |
| `mcp_servers.url` | `connectors.endpoint` | HTTP/SSE only, after userinfo, query, and fragment are removed. A path segment that looks like a key is not stored. |
| `mcp_servers.timeout_sec` | `connectors.timeout_ms` | Seconds times 1000. Null is 0. Negative or over 86400 is `invalid-timeout`. |
| `mcp_servers.enabled` | `connectors.enabled` | Not copied. The inserted row is `false`. |
| `mcp_servers.api_key` | not stored | Presence is a bool. The value is dropped. See secrets. |
| `mcp_agent_grants.server_id` | `agent_connectors.connector_name` | The connector already mapped from that source id. |
| `mcp_agent_grants.agent_id` | `agent_connectors.agent_id` | An agent in `--tenant` whose id equals this value, else whose `agent_key` equals it. The importer does not create agents. |
| `mcp_agent_grants.enabled` | link or no link | `true` inserts a link. `false` does not. An existing link is not deleted. Missing `enabled` uses the column default, true. |

## Secrets

`mcp_servers.api_key` is a secret. Headers, env, and a URL userinfo or query string are treated as secrets too. Decode keeps a presence flag and the cleaned endpoint. It does not keep the secret string on the decoded export.

On insert:

- `credential_ref` is empty.
- `enabled` is false.
- The secrets table is not written.
- No log line, error, or report field contains the secret value.

The report field `needs_credential` lists each accepted server (inserted, or already mapped by source id) that had a non-empty `api_key`, a non-empty `headers` or `env` object, or a URL userinfo or query. The fields are the export tenant's goso tenant, the source id, the source server name, and the goso connector name. The reason is `needs-credential`. A rejected server is not listed, so a name collision does not ask the operator to type a credential into someone else's connector.

A rerun lists the same servers again when the export still has a secret. The importer does not read the secrets table to see whether an operator already re-entered one.

`mcp_user_credentials` and `mcp_context_credentials` are not imported. The report counts them as `credential-table`.

## Not imported, needs decision

These have no field on the goso connector, the agent link, or a per-tenant tool row. They are counted and not written. The report reason is `needs-decision`.

| Source | Why it has no home |
|---|---|
| `mcp_servers.display_name` | `connectors` has no display name. |
| `mcp_servers.command`, `args` | Safe text is part of the stdio endpoint. `--token`, `--api-key`, `--access-token`, `--auth`, `--key`, `--secret`, `--password`, a `KEY=VALUE` pair, a standalone token of 24 or more characters, a JWT (dots), or a Telegram-style token (colon) is not stored. The same shapes in a URL path, including `/s/<jwt>/mcp`, are not stored. The server is listed under `needs_credential`. |
| `mcp_servers.headers` | No header column. Values are dropped. A non-empty object also lists the server under `needs_credential`. |
| `mcp_servers.env` | No env column. Values are dropped. A non-empty object also lists the server under `needs_credential`. |
| `mcp_servers.tool_prefix` | No prefix column. |
| `mcp_servers.settings` | No settings column. |
| `mcp_servers.created_by` | No created-by column. |
| `mcp_servers.created_at`, `updated_at` | `CreateConnector` sets `created_at` to now. There is no updated-at column. |
| `mcp_agent_grants.tool_allow`, `tool_deny`, `config_overrides` | `agent_connectors` cannot narrow tools. A grant with any of these set is not linked. |
| `mcp_agent_grants.granted_by`, `created_at` | The link has no grantor and no timestamp. |
| `mcp_agent_grants.id` | The link key is `(agent_id, connector_name)`. The grant id is not stored. |
| `mcp_user_grants` (whole table) | There is no per-user connector grant. The table is not attached to every agent. |
| `builtin_tools` (whole table) | The catalog is compiled. `tool_flags` is global, so a tenant row would change every tenant. |
| `builtin_tool_tenant_configs` (whole table) | There is no per-tenant tool table. `agent_tool_flags` is per agent and would overwrite. |

`mcp_servers.api_key` is not an open decision. It is dropped on purpose (`dropped-secret`) and the server is listed under `needs_credential`.

## Behavior

1. `goso-mcp-import` takes `--export`, `--tenant`, `--source-tenant`, `--deny-file`, and `--db`. `--source-tenant` is required. `--deny-file` or `GOSO_IMPORT_DENY_FILE` is required. A missing, empty, or malformed deny file stops the command before the database is opened, including a dry-run. The report field `denied_tenants` is a count. Denied tenant ids are not printed.
2. Dry-run is the default. It prints the same per-tenant counts and does not call `CreateConnector` or `LinkAgentConnector`. `--apply` writes. `--apply` together with `--dry-run` is an error. `--dry-run=false` without `--apply` writes nothing and does not open the database.
3. `--db` is the only database. A Postgres host other than `localhost`, `127.0.0.1`, or `::1` is refused. `GOSO_DATABASE_URL` is not opened. Opening `--db` still runs the existing migrations.
4. A second run of the same source id does not insert another connector and does not change the stored name, endpoint, enabled flag, credential, or `created_at`. A different source name in a later file does not create a second row.
5. If the computed name already belongs to a connector and the stored row differs, the reason is `native-exists`. That connector is not updated. Grants for that source id are `server-not-owned` and are not linked.
6. Two source rows in one file that slug to the same name: the first is inserted, the second is `native-exists`.
7. An agent id that exists only in another tenant is `agent-other-tenant` and is not linked. A missing agent is `agent-missing`. The importer does not create agents.
8. For each tenant section, `input = inserted + skipped + rejected + not_imported`. The importer returns an error if a tally does not balance. Logs and the report do not contain secret values.

## Reasons

| Code | When |
|---|---|
| `unchanged` | the source id is already mapped, or the agent link already exists |
| `duplicate` | a later row repeats a source id or a server/agent pair in this file |
| `foreign-tenant` | `tenant_id` is not the selected source tenant |
| `native-exists` | a connector already occupies the target and differs from this export; it is not updated |
| `needs-credential` | the server was not stored because an argument or URL path looks like a credential, or an accepted server still needs a credential re-entered |
| `server-not-owned` | the grant's server was `native-exists` and was not accepted |
| `server-missing` | the grant's server was not accepted |
| `agent-missing` | no agent in `--tenant` has that id or agent key |
| `agent-other-tenant` | that id or agent key belongs to another tenant |
| `grant-disabled` | the grant is disabled and no link exists yet |
| `invalid` | the row could not be read, or `enabled` had a bad type |
| `invalid-id` | the source id or agent ref is empty or not an id token |
| `invalid-name` | the server name is empty or too long |
| `invalid-transport` | the transport is not one of the mapped values |
| `invalid-endpoint` | stdio has no command, or the HTTP URL is not http(s) |
| `invalid-timeout` | `timeout_sec` is not an integer from 0 to 86400 |
| `control` | a kept field contains a control character |
| `needs-decision` | the table or column has no goso home |
| `dropped-secret` | `api_key` was present and was not stored |
| `credential-table` | `mcp_user_credentials` or `mcp_context_credentials` |

## Tests

`TestMinhAnMemory` and `TestMinhAnSQLite` load `testdata/minh-an.json`. Dry-run leaves the pre-seeded `zalocrm` connector and the `web_search` tool flag untouched and writes no link. Apply inserts three disabled connectors, strips the search URL query, leaves `credential_ref` empty, and links only `tu-van-sales` to notes. The other-shop agent is not linked. The ledger server is not inserted. A second apply does not add a row and does not change `created_at`. The SQLite file does not contain `placeholder-minh-an`.

`TestExportDropsSecrets` dumps the decoded fixture and fails if a placeholder remains. `TestDryRunDoesNotWrite` fails if `CreateConnector` or `LinkAgentConnector` is called. `TestNameTakenDoesNotOverwrite` keeps a pre-existing connector's endpoint, enabled flag, and credential ref. `TestSourceIDRenameDoesNotDuplicate` changes the export name and still has one connector. `TestDisabledGrantDoesNotDeleteLink` leaves a hand-made link in place. `TestUserinfoStrippedAndListed` stores the URL without userinfo and lists the server. `TestJSONL` reads a three-line file. `TestCLIDryRunWritesNothingThenApplyIsIdempotent` and `TestCLIRefusesWriteWithoutApply` cover the command, including a non-local DSN whose password must not appear on stderr.

Postgres runs when `GOSO_TEST_POSTGRES_DSN` is a loopback DSN. It is skipped otherwise, and required when `CI` is set. This importer does not install or use pgvector.

## Non-goals

No control-plane change. No new table or column. No pgvector. No MQ Capital fixture. No copy of GoClaw source. Memory, vault, contacts, skills, cron, and channels are not written. The importer does not dial an MCP server and does not enable one.

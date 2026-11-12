# SPEC 174 — GoClaw agent import

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

Sample data is Minh An only. This spec does not read a live GoClaw database, does not copy GoClaw code, and does not add pgvector. The input is an export file whose keys are GoClaw column names.

## Number

| Number | Where it is used |
|---|---|
| 172 | Open PR #46: offline contacts import. |
| 173 | Open PR #45: offline context-file import. |
| 174 | This spec. Agents, teams, members, and links. |

## Why

Goso already has agents, teams, team members, and one-way agent links. A GoClaw export of those tables can be added into that model. Shares, per-user overrides, per-user profiles, and config permissions have no goso table. This spec does not add one.

## Command

```bash
go run ./gateway/cmd/goso-agent-import \
  --export export.json \
  --source-tenant <goclaw-tenant-id> \
  --tenant <goso-tenant> \
  --db /path/to/local.db
```

The default is a dry-run. It prints counts and writes nothing. `--apply` writes. `--apply` together with `--dry-run` is an error. `--dry-run=false` without `--apply` still writes nothing.

`--db` is the only database. A non-local Postgres host is refused. `GOSO_DATABASE_URL` is not opened. Opening `--db` still runs the existing store migrations. The dry-run does not create `goclaw_agent_import_map`.

```bash
go run ./gateway/cmd/goso-agent-import --rollback --tenant <goso-tenant> --db /path/to/local.db
```

`--rollback` without `--apply` counts mapped rows. `--rollback --apply` deletes them. It does not take `--export`.

The file is a JSON object of arrays, or JSONL with `_table` or `table` on each line. Arrays use the GoClaw table names. Unknown keys on a row are ignored. A file over 32 MiB is rejected.

## What is written

Only rows whose `tenant_id` equals `--source-tenant` are eligible. Every other tenant is `tenant-mismatch` and is not written. Eligible rows are inserted into `--tenant` only.

A source id that is already in the map for that destination tenant is `unchanged`. The stored display name, instructions, model, provider, enabled flag, team name, member role, and links are not updated. A rerun does not insert a second row.

`agents.agent_key` is unique across all goso tenants. If any existing agent already has that key, the row is `agent-key-taken`. That agent is not modified and is not adopted into the map, so a later rollback cannot delete it.

A team, member, or link is created only when both ends are agents this import owns. A pre-existing agent is not given a new team membership or a new link.

`GOSO_LITE` caps still apply. A cap stops the run. Rows already mapped stay mapped.

## Column mapping

### `agents` → `agents`

| GoClaw column | Goso | Rule |
|---|---|---|
| `id` | `goclaw_agent_import_map.source_key` | Goso assigns a new `agents.id`. The map stores it. |
| `tenant_id` | not copied | Selects the export tenant. The stored tenant is `--tenant`. |
| `agent_key` | `agents.agent_key` | Trimmed, 1–100 runes, no slash or control character. |
| `display_name` | `agents.display_name` | Trimmed, at most 255 runes. |
| `provider` | `agents.llm_provider` | At most 50 runes. |
| `model` | `agents.model` | At most 200 runes. |
| `status` | `agents.enabled` | Empty or `active` is enabled. `inactive`, `disabled`, `paused`, and `archived` are stored disabled. `deleted` is skipped. Any other value is rejected. |
| `deleted_at` | not stored | A non-empty value skips the row (`deleted`). |
| `frontmatter` | `agents.instructions` | Stored as the prompt text, at most 32 KiB. It is not prefixed, wrapped, or merged with other columns. |
| `created_at`, `updated_at` | not copied | `CreateAgent` stamps the time. |

Not imported from `agents`, because goso has no column and the value is not copied into `instructions` or `display_name`:

| Column | Why it is left out |
|---|---|
| `owner_id` | No owner column on `agents`. |
| `context_window`, `max_tool_iterations`, `max_tokens`, `thinking_level` | No column. |
| `workspace`, `restrict_to_workspace` | Workspace is the process env, not a per-agent column. |
| `is_default`, `agent_type` | `agent_type` is not `orchestration_mode`. |
| `emoji`, `agent_description` | No column. Description is not the prompt. |
| `self_evolve`, `skill_evolve`, `skill_nudge_interval` | Not imported, needs decision. Turning on goso auto-adapt would change runtime behavior. |
| `budget_monthly_cents` | No column. |
| `tools_config`, `sandbox_config`, `subagents_config`, `memory_config`, `compaction_config`, `context_pruning`, `other_config`, `reasoning_config`, `workspace_sharing`, `chatgpt_oauth_routing`, `shell_deny_groups`, `kg_dedup_config`, `model_fallback` | No column. These blobs can hold secrets. They are not parsed and not copied. |
| `embedding`, `tsv` | No pgvector on agents. The values are not read into a string that is stored or logged. |

### `agent_teams` → `teams`

| GoClaw column | Goso | Rule |
|---|---|---|
| `id` | map `source_key` | Goso assigns `teams.id`. |
| `tenant_id` | not copied | Same tenant filter. Stored tenant is `--tenant`. |
| `name` | `teams.name` | Required, at most 255 runes. |
| `lead_agent_id` | `teams.lead_agent_id` | Must be an agent this import owns. `CreateTeam` also inserts that lead as a member. |
| `status` | not stored | Empty or `active` is imported. Any other status is `inactive` and is not written, because a goso team has no status and would look live. |
| `description`, `settings`, `created_by`, `created_at`, `updated_at` | not stored | No columns. |

### `agent_team_members` → `team_members`

| GoClaw column | Goso | Rule |
|---|---|---|
| `id` | map `source_key` | |
| `tenant_id` | not copied | |
| `team_id` | `team_members.team_id` | The team must be one this import owns. |
| `agent_id` | `team_members.agent_id` | The agent must be one this import owns. |
| `role` | `team_members.role` | Empty defaults to `member`. `lead` and `member` only. A membership that already exists is not given a new role. A second lead is not written. |
| `joined_at` | not stored | No column. |

### `agent_links` → `agent_links`

| GoClaw column | Goso | Rule |
|---|---|---|
| `id` | map `source_key` | |
| `tenant_id` | not copied | |
| `source_agent_id`, `target_agent_id` | `from_agent_id`, `to_agent_id` | Both agents must be ones this import owns. A row that points at itself is rejected. |
| `direction` | one or two edges | Empty or `outbound`: source → target. `inbound`: target → source. `both` or `bidirectional`: both edges. Any other value is rejected. |
| `status` | not stored | Empty or `active` is imported. Any other status is `inactive` and is not written. |
| `description`, `max_concurrent`, `settings`, `created_by`, `created_at`, `updated_at`, `team_id` | not stored | A goso link is only the two agent ids. `team_id` is not kept. |

An edge that already exists and is not in the map is `link-exists`. It is not added to the map, so rollback does not remove it. `DeleteAgent` still drops every link that touches an agent rollback deletes. That is the store's delete, and it includes a link an operator added later.

## Not imported, needs decision

These tables are counted per source tenant with reason `needs-decision`. No row is written.

| Table | Why |
|---|---|
| `agent_config_permissions` | Goso has no per-user permission on agent config. Tool flags and evolution guardrails are a different model and are not inferred from `scope` or `permission`. |
| `user_agent_overrides` | Goso model and provider overrides belong to a session, not to a user plus an agent. Creating a session would invent a chat. |
| `user_agent_profiles` | There is no per-user workspace profile. Context files are SPEC 173. This importer does not write `context_files`. |
| `agent_shares` | There is no agent share or ACL table. |

Any other table in the file, such as `skills`, is `not-this-spec` and is not written. Memory, vault, contacts, and knowledge-graph rows are not part of this command.

## Report

Stdout is JSON. Each imported table has one section per source tenant:

`input = inserted + updated + skipped + rejected`

`updated` is always 0. Reason counts sum to `skipped + rejected`. A tally that does not balance is an error and the command exits 1. Stdout and stderr do not include prompt text, display names, user ids, jsonb, or embeddings.

## Rollback

`--rollback --apply` deletes, for `--tenant` only:

1. Mapped link edges whose agents are both in that tenant.
2. Mapped teams. `DeleteTeam` also deletes that team's members, tasks, and messages.
3. Mapped member rows on a team that was not deleted.
4. Mapped agents. `DeleteAgent` also deletes that agent's sessions and the links that touch the agent.

The map rows for a deleted target are removed. A mapped agent that is still the lead of a team this importer did not create stops the rollback. That agent stays, and so does its map row. A second rollback after the extra team is gone removes the agent.

Rows that were never mapped are not deleted. A manual delete of an imported agent leaves the map until rollback. Rollback then drops the stale map and does not recreate the agent. Import it again after that rollback.

## Tests

`TestMinhAnFixture` on memory, SQLite, and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set): dry-run writes nothing and does not create the map; apply inserts the Minh An rows; a second apply and a later dry-run change no id, text, or timestamp; tenant B and a pre-existing agent, link, and memory in the destination tenant stay; rollback removes only the mapped rows; a second rollback removes nothing. The fixture's secret, embedding, description, and emoji markers are absent from the report and from stored fields.

Also: a changed `frontmatter` does not overwrite; a key owned by another tenant is not touched; rollback stops while the agent leads a team this tool did not create; an empty link direction is outbound; JSONL loads one agent and counts a share; a bad export or a bad tenant writes nothing; two concurrent applies insert four agents, not eight; an edge that already exists is not adopted into the map.

## Non-goals

No control-plane change. No new product table for shares, overrides, profiles, or permissions. No copy of GoClaw. No customer export in the tree. No `.github` change. No memory-schema change.

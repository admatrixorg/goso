# SPEC 177 — Import cron jobs and channel instances

Offline importer from a GoClaw export file into goso's existing cron jobs and channel config. No live database connection to GoClaw. No GoClaw code. Sample data is Minh An only.

The command is `goso-cron-channel-import`. The default prints per-tenant counts and writes nothing. `--apply` writes. `--apply` together with `--dry-run` is refused.

## What is stored

Every imported cron job has `enabled` false. Every imported channel config has `enabled` false. The tool does not start the cron runner and does not start a channel.

`channel_instances.credentials` is never written, never logged, and never printed. Each channel instance is listed on the report with `needs_credential: true`.

A second `--apply` of the same source id inserts nothing. The map is `import_source_ids (tenant_id, kind, source_id) → goso id`. An existing goso cron job or channel row is not updated and not deleted.

## Column mapping

### cron_jobs → cron_jobs

| Export column | Goso |
|---|---|
| `id` | `import_source_ids.source_id` (`kind=cron_job`). The goso job id is new. |
| `tenant_id` | Selects the run. The job is attached to a session that already belongs to `--tenant`. |
| `schedule_kind=cron` + `cron_expression` | `spec`, when the existing 5-field parser accepts it. |
| `schedule_kind=every` or `interval` + `interval_ms` | `spec` `every:Nh` or `every:Nm`, when the milliseconds are an exact number of hours or minutes, at least 1. |
| `payload` string, or `payload.message` / `text` / `prompt` / `content` / `body` | `message`. One string. |
| `enabled` | Stored as false. The export value is ignored. |
| `agent_id` | Not a cron column. Used only to pick one existing session of that agent in `--tenant`. |

The destination tenant must already have the session. This tool does not create sessions. If the agent id exists only in another tenant, the job is not imported. If the agent id is unknown and the destination tenant has exactly one session, that session is used.

### channel_instances → channel_config

| Export column | Goso |
|---|---|
| `id` | `import_source_ids.source_id` (`kind=channel_instance`). |
| `tenant_id`, `channel_type`, `name` | `channel_config.name` = `<dest tenant>/<catalog>/<instance name>`. |
| `channel_type` | Catalog name when it is one of `telegram`, `zalo-personal`, `zalo-oa`, `discord`, `slack`, `feishu`, `whatsapp`. Underscores are accepted (`zalo_personal`). |
| `agent_id` | `agent_id` when that agent is already in `--tenant`. |
| `config.dm_policy`, `group_policy`, `require_mention`, `allow_from` | The same fields, when the values match the existing policy enums and the allow list is short ids. |
| `enabled` | Stored as false. The export value is ignored. |
| `credentials` | Not stored. The report lists the instance as needs credential. |

`channel_config` is one global row per name, and the live adapters look up the seven catalog names. The qualified name keeps the import off that lookup, so a row already named `telegram` is not overwritten and this tool cannot start it.

## Not imported, needs decision

These export columns have no field on the goso cron job or channel config:

- cron: `user_id`, `name`, `timezone`, `run_at`, `delete_after_run`, `next_run_at`, `last_run_at`, `last_status`, `last_error`, `created_at`, `updated_at`, `team_id`, `stateless`, `deliver`, `deliver_channel`, `deliver_to`, `wake_heartbeat`, `agent_id`, and payload fields other than the one message string.
- channel: `credentials`, `display_name`, `created_by`, `created_at`, `updated_at`, and config fields other than `dm_policy`, `group_policy`, `require_mention`, `allow_from`.

A whole row is also reported `not imported, needs decision` when:

- the schedule is `at` / one-shot, or the cron expression or interval is outside the existing parser
- the channel type is not one of the seven catalog names (`zcrm` included)
- the destination tenant has no single session to attach the job to
- the cron cap (20) is already full
- the qualified channel name is already in use (the existing row is left as it is)

How a disabled per-tenant instance later becomes the one live catalog row is not decided here. Credentials stay out until an operator enters them through the existing env or secrets path.

`cron_run_logs` is not a source table for this tool. A line for any other table is counted and not written.

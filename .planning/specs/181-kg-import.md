# SPEC 181 — KG import (GoClaw kg_entities / kg_relations)

Status: implemented on this branch. One PR into `main`. This is PR C of the CTO memory-import design (revision 2, 12/11 20:59 UTC+7). SPEC 182 (customer_key on the Goso KG) is a separate PR and is not in this one.

Sample data is Minh An only (`minh-an`, `phap-che`). No customer export, no phone number, no secret, and no GoClaw source. The importer does not name a blocked tenant.

## Why

Goso already has `kg_entities` and `kg_relations` (`gateway/internal/store/postgres.go`, `sqlite.go`). GoClaw's graph is a different shape. This importer writes the rows Goso can store today: tenant-wide entities and the relations whose endpoints are also tenant-wide. About 95% of the GoClaw graph is customer-bound. Those rows wait for SPEC 182.

## GoClaw columns (read 2026-11-12)

`kg_entities`: `id`, `agent_id`, `user_id` (varchar, not null, default `''`), `external_id`, `name`, `entity_type`, `description`, `properties`, `source_id`, `confidence`, `created_at`, `updated_at`, `team_id` (uuid, null), `embedding`, `tenant_id`, `tsv`, `valid_from`, `valid_until`. There is no `custom_scope` column.

`kg_relations`: `id`, `agent_id`, `user_id` (varchar, not null, default `''`), `source_entity_id`, `relation_type`, `target_entity_id`, `confidence`, `properties`, `created_at`, `team_id` (uuid, null), `tenant_id`, `valid_from`, `valid_until`. There is no `description` and no `custom_scope`.

Not copied: `embedding`, `tsv`, `confidence`, `external_id`, `source_id`, `updated_at`. The insert lists Goso's existing columns only. It does not add `customer_key` and it does not write `kg_entities.embedding`.

## Behavior

1. Input is a JSONL file (`--export`). The tool does not open a GoClaw database. A `postgres://` export path is refused. `--db` is a local SQLite file or a loopback Postgres DSN. `GOSO_DATABASE_URL` is not opened.
2. Each line is one object with `"table": "kg_entities"` or `"kg_relations"` and the column names above. Unknown fields, including `embedding` and `tsv`, are dropped. A line that is not JSON fails the run before any write.
3. `--source-tenant` and `--tenant` are required. The tenant map is the SPEC 152 pairing, and this run writes only that one source into that one dest. `--deny-file`, or `GOSO_TENANT_DENY_FILE` when the flag is empty, is required. The file is a local list of tenant ids, one per line or `{"tenants":[...]}`. A missing, empty, or malformed file stops before the database is opened, including on `--dry-run`. If the source tenant or `--tenant` is on the list, the run stops before any write and exits non-zero. The report field `denied_tenants` is a count. Denied tenant ids are not printed.

```json
{"tenants":[{"source_tenant":"<goclaw tenant uuid>","dest_tenant":"minh-an","slug":"minh-an"}]}
```

4. Agent map (`--agent-map`) is an input file. SPEC 174 (the agents importer) is a separate PR. This PR does not import that code. The Goso agent must already exist in the destination tenant. `source_tenant`, when set, applies the row only to that GoClaw tenant.

```json
{"agents":[{"goclaw_agent_id":"<uuid>","goso_agent_id":"<goso id>","source_tenant":"<goclaw tenant uuid>"}]}
```

5. A non-empty `user_id` is resolved by `ResolveCustomerKey` to `<channel>:<id>` from the PR #46 map shape, or `goclaw:<user_id>` when the map has no row. The key is never empty for that user and is not printed. The CLI does not take a customer-map flag, because the key is not written. Imported rows leave the SPEC 182 `customer_key` column at its default `''`. An entity is written only when `admit` returns empty. `admit` is the only user/team/scope decision:
   - non-empty `team_id`, or a non-empty `custom_scope` if a line carries one, is `team/scope` and is not written;
   - otherwise a non-empty `user_id` is `customer-private` and is not written;
   - JSON `null` `user_id` is empty, matching the column default `''`.
   There is no second import path for team or scope. The `team/scope` counter stays so a later export that sets them is visible. Counts on 12/11 were zero for `team_id`.
6. A relation is written only when its own `user_id` is empty and both endpoints were kept in the same source tenant with an empty `user_id`. A tenant-wide relation whose endpoint was held as customer-private (or was missing, or was held for another reason) is `missing relation endpoint`. A relation whose own `user_id` is set is `customer-private`, not also an endpoint skip.
7. Written entity: `id` `gc-kg:<uuid>`, `kind` = `entity_type`, `body` = trimmed description, then a newline and compact JSON `properties` when both are present. `{}` and null properties add nothing. `agent_id` is the Goso agent. `provenance` is `goclaw:<uuid>`. `valid_from` null is stored as `created_at`. When `created_at` is null and `valid_from` is set, `created_at` is that same instant. Both null is `invalid`. `valid_until` null stays null. A past `valid_until` is kept. The `expired` counter stays 0.
8. Written relation: `id` `gc-kg:<relation uuid>`, `rel` = `relation_type`, `from_id` / `to_id` are `gc-kg:<entity uuid>`. Body is compact `properties` only, because the table has no description. Provenance is `goclaw:<relation uuid>`. The same `valid_from` rule applies.
9. Inserts are `ON CONFLICT (id) DO NOTHING`. A stored row with the same contents is `exists` and is not updated. A second run's `will_write` is 0. A stored row with different contents is `native-exists` and is not updated, and it is not used as a relation endpoint.
10. Skip priority for one row: unmapped tenant, `team/scope`, `customer-private`, `invalid`, duplicate id (`exists`), unmapped agent, then for a relation `missing relation endpoint`, then an identical stored row (`exists`) or a different stored row (`native-exists`). A tenant on the denylist is not a section in the report.
11. `--dry-run` prints the same counters and writes nothing. For each source tenant and table, `read = will_write + sum(skipped)`. The skipped keys are `expired`, `customer-private`, `team/scope`, `unmapped`, `missing relation endpoint`, `exists`, `native-exists`, `invalid`.
12. `--remove --tenant <goso>` deletes rows in `kg_relations` and `kg_entities` for that tenant whose id starts with `gc-`. The match is case-sensitive (`GLOB` on SQLite, `LIKE` on Postgres). Other ids, other tenants, and other tables stay. The same denylist stops a listed tenant before any delete. `--dry-run` counts the deletion and does not delete.

## Follow-up after SPEC 182

`admit` in `gateway/internal/kgimport/scope.go` is the gate. The inserts in `gateway/internal/store/kg_import.go` take no customer key and do not add a column. After SPEC 182 adds `customer_key`, the follow-up changes `admit` so a non-empty `user_id` can be kept, passes that key into the insert, and updates `endpointKept` for relations that point at those entities. `team/scope` stays a skip. This PR does not write those rows and does not add the column.

`GetKGEntity` still folds an unknown provenance string to `posted` or `extracted`. The stored column is `goclaw:<uuid>`. `KGProvenance` reads that column without the fold.

## Tests

`TestMinhAnFixture` on memory and SQLite (Postgres when `GOSO_TEST_POSTGRES_DSN` is set): dry-run writes nothing; the Minh An counters match; a private entity, a team row, a `custom_scope` row, an unmapped agent, and a bad id are absent; a relation to a private entity is `missing relation endpoint`; a second run inserts nothing; tenant `minh-an` does not see `phap-che`; removal deletes `gc-` rows and leaves a live entity, a live relation, a live memory, a `GC-` id, a `gc-sess:` memory, and the other tenant. `TestConflictDoesNotOverwrite` keeps a pre-seeded body and counts `native-exists`. `TestDenyFileAndDeniedTenant` checks an empty or malformed denylist and a listed source or dest that writes nothing and is not named. `TestAdmitKeepsCustomerAndTeamOut` locks the gate. The command test checks the CLI, a second run, removal, and a missing, empty, malformed, or blocking denylist.

## Non-goals

No control-plane change. No schema change. No pgvector. No customer_key. No import of `memory_chunks`, episodic summaries, or vault documents. No copy of GoClaw. No customer file in the tree.

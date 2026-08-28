# SPEC 072 — Memory L2 knowledge graph + progressive load (FTS5)

> LOCKED after SPEC 071 merge `1974e57`. Clean-room. Do not kill `:8082` `:8091`.
> pgvector **host** stays DI-09 (`docs/qa/071-pgvector-path.md`). Do not block on it.

Closes matrix **M3, M4, K4** (L2 KG + progressive load + `knowledge_graph_search` / expand). FTS5/Lite first.

## GoClaw (cite, no copy)

Source: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC).

| Behavior | Cite |
|----------|------|
| L2 semantic memory = KG `kg_entities` + `kg_relations` with temporal validity | `docs/07-bootstrap-skills-memory.md` §17 |
| Progressive tools: auto-inject abstracts; `memory_search` L1+L2; `memory_expand(id)` deep retrieve | same §17 Progressive Tool Access |
| Hybrid FTS + vector; vector is a **host** concern | same doc hybrid search; 071 PG path |

## goso today

- L0 messages + L1 episodic (`docs/qa/036-memory.md`). FTS5 `memory_fts` (D5).
- No `kg_entities` / `kg_relations`. No `memory_expand`. Search is FTS over memories/messages only (K4 PARTIAL).

## goso plan (self-written)

1. SQLite tables (CREATE IF NOT EXISTS + tenant_id DEFAULT `default` from 071):
   - `kg_entities(id, tenant_id, name, kind, body, valid_from, valid_until, created_at)`
   - `kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until)`
   - FTS5 virtual table on entity/relation `body`+`name` (reuse `memory_fts` pattern; do not require pgvector).
2. HTTP (goso-shaped `/api`):
   - `POST /api/kg/entities` `{name, kind, body}` (admin).
   - `GET /api/kg/search?q=` FTS over L1 memories **and** L2 entities (progressive: hits are ids + snippet + `tier: l1|l2`).
   - `GET /api/kg/entities/{id}` expand: entity + relations + linked names (deep retrieve).
3. Tools (pipeline ToolSpec, fail-closed if empty query): `memory_search`, `memory_expand`. LLM-driven only (no keyword matchTools).
4. Optional extract: after chat, if `GOSO_KG_EXTRACT=1`, a tiny heuristic (Name: / Entity:) may insert an entity from the assistant text — tests use an explicit POST, not a live LLM. Default **off** so demo chat is unchanged.
5. Memory page (or existing Memory UI): search box + expand panel. i18n vi+en. StatusLine.
6. Isolation: tenant filter when `GOSO_MULTI_TENANT=1` (071). Demo: default tenant only.

## Tests

- Insert two entities; FTS query hits the named one; unrelated query empty.
- Expand returns relations.
- `memory_search` / `memory_expand` tools via scripted provider ToolCalls.
- Existing L0/L1 tests still pass. Demo path no header.

## Non-goals

pgvector embeddings. Live Postgres. Copying goclaw schema names into HTTP (`/v1` alias of `/api` is OK). Binding demo ports.

QC: typecheck, `go test ./...`, build, agpl, agpl-docs.
`docs/qa/072-memory-l2-fts.md` with the cite table.
Commit `admatrixmdp/spec072-memory-l2-fts`. Do not merge. Do not start 073.

# QA — SPEC 072 Memory L2 knowledge graph + progressive load (FTS5)

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not wait for a pgvector/Postgres host (DI-09 parked). Do not merge. Do not start SPEC 073.

Closes matrix **M3, M4, K4** on FTS5/Lite. Vector embeddings stay DI-09 (`docs/qa/071-pgvector-path.md`).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| L2 semantic memory = KG `kg_entities` + `kg_relations` with temporal validity (`valid_from`, `valid_until`) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` §17 |
| Progressive tools: auto-inject abstracts; `memory_search` L1+L2; `memory_expand(id)` deep retrieve | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` §17 Progressive Tool Access |
| Hybrid FTS + vector; vector is a **host** concern | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` (hybrid search); pgvector path `docs/qa/071-pgvector-path.md` |

goso mapping (self-written): SQLite `kg_entities` + `kg_relations` with `tenant_id TEXT NOT NULL DEFAULT 'default'` (same 071 default). FTS5 virtual table `kg_fts` on entity `name`+`body` and relation `rel`+`body`. Progressive search is lexical L1 memories/messages **and** L2 entities (`tier: l1\|l2`). Expand returns the entity, current relations, and linked names. Tools `memory_search` / `memory_expand` are pipeline ToolSpecs, advertised always, invoked only from LLM ToolCalls (no keyword matchTools). Optional `GOSO_KG_EXTRACT=1` may insert an entity from assistant lines `Name:` / `Entity:`; default **off**. Tests use explicit `POST /api/kg/entities`. Demo / unset multi-tenant stays `default` (no header).

## What changed

- Store (in-memory + SQLite): `kg_entities(id, tenant_id, name, kind, body, valid_from, valid_until, created_at)`, `kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until)`, FTS5 `kg_fts`. Search falls back to `instr()` if FTS cannot be created.
- HTTP (Bearer like other `/api/*`; `/v1` alias of list/search/POST):
  - `POST /api/kg/entities` `{name, kind, body}` → 201.
  - `POST /api/kg/relations` `{from_id, to_id, rel, body?}` so expand has edges.
  - `GET /api/kg/search?q=` FTS over L1 + L2; hits `{id, snippet, tier, kind?, name?, session_id?}`. Empty `q` → **400**. No hits → `[]`.
  - `GET /api/kg/entities/{id}` expand: entity + relations + linked names. Wrong tenant → 404.
- Tools: `memory_search({query})` and `memory_expand({id})`. Fail-closed on empty query/id. Tenant from the calling agent.
- `GOSO_KG_EXTRACT` default off. When `1`, after chat, a tiny `Name:` / `Entity:` heuristic may insert an entity. Not a live LLM extract.
- Memory UI: progressive search box + L2 expand panel. i18n vi+en. StatusLine.
- Isolation: tenant filter when `GOSO_MULTI_TENANT=1`. Demo: default tenant only.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 073.

## Proof

- Two entities; FTS query hits the named one; unrelated query empty (`TestStore_KGSearchAndExpand`, `TestSQLiteStore_KGFTSAndExpand`, `TestKGAPI_EntitiesSearchExpand`, `TestKGAPI_SQLiteFTS`).
- Expand returns relations and linked names.
- `memory_search` / `memory_expand` via scripted provider ToolCalls (`TestChat_MemorySearchExpandTools`). Empty query fail-closed.
- Existing L0/L1 tests still pass. Demo path no header (`tenant_id=default`).
- Header isolation when `GOSO_MULTI_TENANT=1` (`TestKGAPI_TenantIsolation`).
- Extract default off; `GOSO_KG_EXTRACT=1` inserts from `Entity:` (`TestChat_KGExtractDefaultOff`, `TestChat_KGExtractOn`).

## Non-goals

pgvector embeddings. Live Postgres. SemanticWorker / dreaming workers. Copying goclaw schema names into HTTP (`/v1` alias of `/api` is OK). Binding demo ports. Merge. SPEC 073. Copying goclaw Go. Secrets in docs.

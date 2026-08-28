# QA — SPEC 075 Skills BM25 search + manage

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not execute skill scripts. Do not wait for pgvector (DI-09). Do not merge. Do not start SPEC 076.

Closes matrix **K6** (`use_skill` loader only → BM25 `skill_search` + jailed create/delete).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| In-memory BM25 skill search, k1=1.2 b=0.75, top 5, lazy rebuild | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` §10 BM25 |
| Inline vs search mode by skill count/tokens | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` §9–10 |
| Hybrid BM25+vector is host; goso ships BM25 only | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` §11 embeddings = DI-09 |
| Skills are SKILL.md folders, searchable, not executed binaries | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/14-skills-runtime.md` (searchable skills); goso 049 already loads markdown |

goso mapping (self-written): tokenize lowercase, drop 1-character tokens; IDF `log((N-df+0.5)/(df+0.5)+1)`; score `IDF × tf × (k1+1) / (tf + k1 × (1-b + b × |d|/avgDL))`. Index fields are folder name + YAML `description` (else first prose line) + body. Rebuild when List/Load sees a newer SKILL.md mtime (or count change). Empty `GOSO_SKILLS_DIR` never walks the filesystem. Tests use a temp dir. Hybrid vector/pgvector stays DI-09.

## What changed

- `GET /api/skills?q=` returns ranked `{name, score, snippet}` max 5. Empty `q` keeps the existing `{name, path}` list. Unconfigured → `{skills:[]}`.
- Builtin `skill_search({query})` advertised with `use_skill`. LLM ToolCalls only (scripted provider in tests). Empty query → `query is required` (no ranking). Empty env → `not_configured`.
- Manage, jailed to `GOSO_SKILLS_DIR`: `POST /api/skills` `{name, body}` writes `<name>/SKILL.md` (overwrite if the name exists); `DELETE /api/skills/{name}`. Name `[a-z0-9_-]{1,64}`. Cap 64KiB. Deny `..`. Never exec.
- Functions Skills card: search box, create (name+body), delete with confirm. i18n vi+en.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 076.

## Proof

- Two skills; query hits the relevant name first; unrelated query empty (`TestSearch_RanksRelevantFirst`, `TestSkills_SearchCreateDelete`).
- POST then GET `?q=` finds it; DELETE then 404 (`TestSkills_SearchCreateDelete`, `TestCreateDelete_TempDir`).
- Path jail on name; empty env no FS walk (`TestCreate_RejectsJailAndSize`, `TestDelete_PathJail`, `TestList_EmptyEnvFailClosed`, `TestSkills_EmptyEnvSearchManageFailClosed`).
- `skill_search` via scripted ToolCalls (`TestChat_SkillSearchToolCalls`). Empty query fail-closed (`TestChat_SkillSearchEmptyQueryFailClosed`).
- Lazy rebuild on newer mtime (`TestSearch_RebuildOnNewerMtime`).

## Non-goals

Hybrid BM25+pgvector (DI-09). Executing skill scripts. MCP `/v1/skills` rewrite. Merge. SPEC 076.

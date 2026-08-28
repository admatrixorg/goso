# SPEC 075 — Skills BM25 search + manage

> LOCKED after SPEC 074 merge. Clean-room. Do not kill `:8082` `:8091`.
> Do **not** execute skill scripts. Do not wait for pgvector (DI-09).

Closes matrix **K6** (`use_skill` loader only; no BM25 / `skill_manage`).

## GoClaw (cite, no copy)

| Behavior | Cite |
|----------|------|
| In-memory BM25 skill search, k1=1.2 b=0.75, top 5, lazy rebuild | `docs/07-bootstrap-skills-memory.md` §10 |
| Inline vs search mode by skill count/tokens | same §9–10 |
| Hybrid BM25+vector is host; goso ships BM25 only | §11 embeddings = DI-09 |
| Skills are SKILL.md folders, not executed binaries | `docs/14-skills-runtime.md` (search); goso 049 already loads markdown |

## goso today

- 049: `GOSO_SKILLS_DIR` one-level `<name>/SKILL.md`, 64KiB, jail, `use_skill`, `GET /api/skills` names. Empty dir fail-closed. No BM25. No create/delete.

## goso plan (self-written)

1. BM25 index over skill **name + description + body** (tokenize lowercase, drop 1-char tokens). Rebuild when List/Load sees a newer mtime. `GET /api/skills?q=` returns ranked `{name, score, snippet}` max 5. Empty `q` keeps the existing name list.
2. Tool `skill_search({query})` — LLM ToolCalls only; empty query fail-closed. Keep `use_skill`.
3. Manage (admin, jailed to `GOSO_SKILLS_DIR`): `POST /api/skills` `{name, body}` writes `SKILL.md` (name `[a-z0-9_-]{1,64}`), `DELETE /api/skills/{name}`. Never exec. Cap 64KiB. Deny `..`.
4. Empty `GOSO_SKILLS_DIR`: search/manage fail-closed `{skills:[]}` / `not_configured`. Tests use a temp dir.
5. Functions Skills card: search box + create (name+body) + delete confirm. i18n vi+en.

## Tests

- Two skills; query hits the relevant name first; unrelated query empty or lower rank.
- POST then GET ?q= finds it; DELETE then 404.
- Path jail on name. Empty env no FS walk.
- `skill_search` via scripted ToolCalls.

QC: typecheck, go test, build, agpl, agpl-docs.
`docs/qa/075-skills-bm25.md` with cite table.
Commit `admatrixmdp/spec075-skills-bm25`. Do not merge. Do not start 076.

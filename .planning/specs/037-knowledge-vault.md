# SPEC 037 — Knowledge Vault (FTS5, no pgvector)

> LOCKED: 2026-08-27. Clean-room. Closes matrix **V1, V2 (FTS half), V3**. **pgvector / L2 KG = DI-09 — do not implement.**
> Demos `:8082` `:8091` `:3000` `:18080` `:18088` — do not kill or bind. No goclaw copy. No banned author ids. No product secrets.

## Goal

Document registry with **`[[wikilinks]]`** (bidirectional edges), **FTS5** search (same engine as SPEC 036), **filesystem sync** (markdown/text on disk; registry stores id, title, path, sha256, mtime). HTTP **`/api/vault`** so MCP vault tools can be retargeted later (do not require `/v1` unless a one-line alias).

## Data

- `vault_docs`: `id`, `title`, `path` (relative to vault root), `sha256`, `mtime`, `body` (optional cache; disk is source of truth after sync)
- `vault_links`: `from_id`, `to_id`, `raw` (`[[Title]]` or `[[id]]`) — **both directions** stored or queryable (`backlinks`)
- Vault root: env `GOSO_VAULT_DIR` (default `data/vault` under cwd). Tests use `t.TempDir()`.
- In-memory store: same API without FTS5 (substring).

## Wikilinks

Parse `[[...]]` in body on put/sync. Resolve by exact title (case-insensitive) then by id. Unresolved links still recorded as raw, `to_id` empty until a matching title appears (re-resolve on sync). `GET /api/vault/docs/{id}/links` → `{outbound, inbound}`.

## Filesystem sync

- `POST /api/vault/sync` walks `GOSO_VAULT_DIR` `*.md` `*.txt` (no `..` escape). Upsert by relative path; skip unchanged sha256; delete registry rows whose files vanished.
- `PUT /api/vault/docs` with `{title, body}` writes `{slug}.md` under vault dir + registry row.
- Never follow symlinks outside root. Tests: write two files with `[[Other]]`, sync, assert edge both ways + FTS hit.

## HTTP (Bearer like `/api/*`)

| Method | Path |
|--------|------|
| GET | `/api/vault/docs` list |
| GET | `/api/vault/docs/{id}` |
| PUT | `/api/vault/docs` create/update by title |
| GET | `/api/vault/docs/{id}/links` |
| GET | `/api/vault/search?q=` FTS5 / substring (empty q → 400, none → `[]`) |
| POST | `/api/vault/sync` |

401 without token when auth on.

## Hybrid search

FTS5 over title+body (SQLite). Do **not** add vector columns. QA must say “lexical only; semantic = DI-09”.

## Non-goals

pgvector, embeddings, browser tool, copying goclaw vault package.

## QC

`go test ./...`, `go build ./gateway/cmd/goso-gateway`, sibling `goso-crm/scripts/agpl-check.sh` 0, `docs/qa/037-knowledge-vault.md`. Commit, do not merge, do not restart demos.

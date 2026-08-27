# QA — SPEC 037 Knowledge Vault (FTS5, no pgvector)

Date: 2026-08-27. Clean-room. Closes matrix rows **V1, V2 (FTS half), V3**. **pgvector / L2 KG = DI-09 — not implemented.** Search is **lexical only; semantic = DI-09**. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

## What changed

Document registry (`vault_docs`) stores `id`, `title`, `path` (relative to vault root), `sha256`, `mtime`, and an optional `body` cache. Disk under `GOSO_VAULT_DIR` (default `data/vault`) is source of truth after sync. Tests use `t.TempDir()`.

`[[wikilinks]]` in body are parsed on PUT and sync. Resolution is exact title (case-insensitive) then id. Unresolved edges keep `raw` with empty `to_id` until a matching title appears (re-resolve on sync/put). `GET /api/vault/docs/{id}/links` returns `{outbound, inbound}`.

Filesystem sync (`POST /api/vault/sync`) walks `*.md` and `*.txt`, rejects `..` escape, does not follow symlinks outside the root, upserts by relative path, skips unchanged sha256, and deletes registry rows whose files vanished. `PUT /api/vault/docs` with `{title, body}` writes `{slug}.md` and upserts the registry.

FTS5 (`modernc.org/sqlite`) indexes title+body. If the virtual table cannot be created, search falls back to `instr()`. In-memory store: case-insensitive substring. Empty `q` → **400**. No hits → `[]`. No vector columns.

HTTP (Bearer like other `/api/*`):

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/api/vault/docs` | list (`{"docs":[…]}`) |
| GET | `/api/vault/docs/{id}` | one doc (disk body when present) |
| PUT | `/api/vault/docs` | `{title, body}` create/update by title |
| GET | `/api/vault/docs/{id}/links` | `{outbound, inbound}` |
| GET | `/api/vault/search?q=` | FTS5 / substring |
| POST | `/api/vault/sync` | walk vault dir |

401 without Bearer when auth is on. `/v1/vault` was **not** added. MCP vault tools are not retargeted in this SPEC.

## Commands

```
go test ./...
gofmt -l gateway
go vet ./gateway/...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
/Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Two files with `[[Other]]` / `[[Alpha]]`, sync, edges both ways + FTS/substring hit (`TestSync_BidirectionalLinksAndSkipDelete`, `TestSync_SQLiteFTSHit`).
- Unresolved `to_id` fills after the target title is written (`TestPut_WritesSlugAndResolves`, `TestStore_VaultWikilinksAndSearch`).
- Unchanged sha256 skipped; vanished files drop registry rows; symlink outside root skipped (`TestSync_*`).
- HTTP list/put/get/links/search/sync + empty `[]` + 400 empty q + Bearer 401 (`TestVaultAPI_*`).
- SQLite FTS over title+body (`TestSQLiteStore_VaultFTSAndLinks`).

## Non-goals

pgvector / embeddings / semantic search (DI-09), browser tool, copying a goclaw vault package, `/v1/vault` alias, MCP retarget.

# QA — SPEC 098 Vault operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Vault: type chips, search, agent/team filters, document list, inbound/outbound links, loading/empty, relationship view with node limit | `docs/qa/090-goclaw-sidebar-ux.md` Vault |

goso mapping (self-written): live tab `vault` in [App.tsx](../../control-plane/src/App.tsx) still renders [VaultPage](../../control-plane/src/pages/VaultPage.tsx). Existing `GET/PUT /api/vault/docs`, `GET /api/vault/search`, `POST /api/vault/sync`, and `GET /api/vault/docs/{id}/links` stay. Untrusted document body is rendered as a React text node inside `<pre>` (no raw HTML inject, no canvas).

Out of scope: Knowledge Graph page (107). Storage page (108). Copying GoClaw canvas. Live vendor tokens. SPECs 099–102.

## What changed

- List: local search plus type/agent/team filters, FTS search hits, loading / empty / filter-empty / error. Type/agent/team come from path prefixes (`agents/{id}/`, `teams/{id}/`, `TEAM.md`) or a leading YAML-like frontmatter block; they are not a new registry column.
- Detail: id, path, type, agent, team, short SHA-256, mtime, inbound/outbound wikilinks (clickable when resolved). Body is plain text, capped at 20_000 characters.
- Sync health: `GET /api/vault/health` compares disk `*.md`/`*.txt` with the registry (`missing_on_disk`, `unindexed`, `hash_mismatch`, `stale`). The page warns when `stale` is true. `POST /api/vault/sync` is unchanged.
- Relationships: `GET /api/vault/graph?limit=` returns a capped node/edge **list** (default 40, max 200). The UI states “no canvas” and falls back to the selected document’s inbound/outbound neighborhood if the graph call is absent.
- i18n vi+en. CP typecheck. Helper tests + vault health/graph HTTP. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/vault ./gateway/internal/httpapi -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `classifyDoc`, `filterVaultDocs`, `plainVaultBody` (HTML stays text, length cap), `isStaleHealth`, `boundNeighborhood` / `normalizeGraph` caps, `capRows` in `vault-ops.ts`.
- `go test` vault + httpapi: health healthy → unindexed extra file → missing after delete; graph returns linked nodes and truncates at `limit=1`; HTTP `GET /api/vault/health` and `GET /api/vault/graph`.
- Vault body is `{plainVaultBody(selected.body)}` in `<pre>`. No `dangerouslySetInnerHTML`. SHA-256 is shortened. Graph copy is “no canvas”.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Knowledge Graph page (107). Storage page (108). Copying GoClaw canvas. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 099–102.

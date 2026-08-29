# QA — SPEC 099 Memory operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No paid embedder.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Memory: Documents / Episodic tabs, agent/scope filters, table + document detail, relation expand, “Embedding: Not configured” | `docs/qa/090-goclaw-sidebar-ux.md` Memory |

goso mapping (self-written): live tab `memory` in [App.tsx](../../control-plane/src/App.tsx) still renders [MemoryPage](../../control-plane/src/pages/MemoryPage.tsx). L1 notes stay on `memories` (`kind=episodic` or `kind=durable`). L2 expand stays `GET /api/kg/entities/{id}`. List JSON omits `body` and returns `snippet` only. Detail `GET /api/memory/{id}` returns the full note as a React text node inside `<pre>` (no raw HTML).

Out of scope: Knowledge Graph page (107). Vault (098). Copying GoClaw tabs. Live vendor tokens / paid embeddings. SPECs 100–102.

## What changed

- List: `GET /api/memory` lists tenant notes without requiring `session_id`. Optional `session_id`, `agent_id`, `kind` filters. Loading / empty / filter-empty / error. When both episodic and durable rows exist, the page splits them.
- Detail: id, scope, kind, agent, session, created_at, full body. L2 relation expand unchanged for `tier=l2` search hits.
- Index: `GET /api/memory/index` reports `{search: fts5|substring, fts, embedding: "not_configured", embedding_configured: false}`. The page warns when embeddings are missing and does not invent a paid embedder.
- Mutations: `POST /api/memory` create (kind `episodic`/`durable`; `document` aliases durable; `message` reserved). `PATCH /api/memory/{id}` edit. `DELETE /api/memory/{id}` delete. Confirm names the snippet/id. Kind `message` is rejected.
- i18n vi+en. CP typecheck. Helper tests + memory HTTP. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/store ./gateway/internal/httpapi -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `normalizeKind`, `memoryLane`, `filterMemories`, `hasBothLanes`, `memorySnippet` / `plainMemoryBody` (HTML stays text, length cap), `isEmbeddingConfigured` stays false for `not_configured`, `listTargetName`, `capRows`.
- `go test` store + httpapi: query by agent/kind, get/update/delete, list omits `body`, reserved `kind=message` is 400, index embedding is `not_configured`, `/v1/memory` list-all matches `/api/memory`.
- Lists render `memorySnippet(...)`. Detail body is `{plainMemoryBody(selected.body)}` in `<pre>`. No `dangerouslySetInnerHTML`. Delete confirm interpolates `{name}`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Knowledge Graph page (107). Vault (098). Copying GoClaw tabs. Live vendor tokens. Paid embeddings. Binding/killing demo ports. Merge. SPECs 100–102.

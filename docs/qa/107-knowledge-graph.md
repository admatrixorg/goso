# QA — SPEC 107 Knowledge Graph explorer

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No paid embedder.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Knowledge Graph: agent/scope selectors, graph workspace, agent-required empty state, embedding not configured | `docs/qa/090-goclaw-sidebar-ux.md` Knowledge Graph |

goso mapping (self-written): live tab `kg` in [App.tsx](../../control-plane/src/App.tsx) renders [KnowledgeGraphPage](../../control-plane/src/pages/KnowledgeGraphPage.tsx). Explorer list is `GET /api/kg/graph?agent_id=&scope=&q=&limit=` (default 40 nodes, max 200; edge cap 2×). Detail expand stays `GET /api/kg/entities/{id}`. Index health is `GET /api/kg/index` (same shape as memory: FTS vs substring; embedding always `not_configured`). Search `GET /api/kg/search` and Memory expand / Vault links stay on their pages.

Out of scope: Storage (108). Copying GoClaw canvas. Live vendor tokens. Paid embeddings. SPECs 108–118.

## What changed

- Live nav tab + page binding in `App.tsx` (work group, after Memory). Agent and scope are required empty states. List is usable without a canvas (`list` default; `graph` is still a node/edge list). Loading / empty / error / not-configured (embedding badge).
- Bounded node/edge counts. Provenance (`source` posted|extracted, `inferred`, created/valid window) is visible. Extracted relationships are labeled “not a fact”; API includes `inferred_are_not_facts: true`.
- Reuse `/api/kg/...`. Graph GET returns snippets only (no `body`). Token-shaped values are dropped. Client `normalizeGraph` / `publicHasSecrets` is a second gate. No invented vendor tokens or paid embedder.
- i18n vi+en. CP typecheck. Tests. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/store ./gateway/internal/httpapi ./gateway/internal/pipeline -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` 115/115 including `normalizeScope`, `isInferred`, `kgSnippet` / `plainKgBody` dropping `sk-` / Bearer, `isEmbeddingConfigured` false for `not_configured`, `normalizeGraph` dropping `api_key` rows and capping nodes.
- `go test` store: agent filter, posted vs extracted scope, secret snippet omitted, cap `limit=1` truncated, sqlite roundtrip of `agent_id`/`provenance`. httpapi: missing `agent_id` 400, unknown agent 404, GET omits `sk-live-` / secret keys, `inferred_are_not_facts` + embedding `not_configured`, `/v1` alias, `/api/kg/index`.
- Page copy: “No canvas” / “Không có canvas”. Extracted badge “not a fact”. Empty state until agent and scope are selected. Expand body is `{plainKgBody(...)}` in `<pre>`. No `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Storage (108). Copying GoClaw canvas. Live vendor tokens. Paid embeddings. Binding/killing demo ports. Merge. SPECs 108-118.

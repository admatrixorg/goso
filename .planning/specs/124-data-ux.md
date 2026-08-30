# SPEC 124 — DATA operator UI/UX

Status: queued after SPEC 123 CTO/advisor QC
Owner: Grok implementation; Codex CTO architecture, merge, and live QC (advisor live-QC while Codex credits exhausted)
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's DATA pages answer the same operator questions and cover the same operational states as live Dewee without copying pixels, wording, CSS, components, or source. Scope is Memory, Vault, Knowledge Graph, and Storage. Reuse the accepted PageChrome / PageStatus / `classifyPageState` / `inventoryBlocksMutation` pattern from SPEC 120–123. Preserve goso's stronger write-only, public-shape, path-confinement, and untrusted-content contracts.

## Live surfaces and APIs

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Memory | `http://127.0.0.1:18791/memory` | App tab `memory`, `control-plane/src/pages/MemoryPage.tsx` | list/get/create/patch/delete `/api/memory*`; index `/api/memory/index`; search `/api/memory/search`; progressive search `/api/kg/search`; expand `/api/kg/entities/:id` via `api/memory.ts`, `api/memory-ops.ts` |
| Vault | `http://127.0.0.1:18791/vault` | App tab `vault`, `VaultPage.tsx` | docs list/get/put `/api/vault/docs*`; links; search; sync; health; graph via `api/vault.ts`, `api/vault-ops.ts`; agent inventory `/api/agents`; teams `/api/teams` |
| Knowledge Graph | `http://127.0.0.1:18791/knowledge-graph` (also `/knowledge`) | App tab `kg`, `KnowledgeGraphPage.tsx` | graph `/api/kg/graph`; index `/api/kg/index`; expand `/api/kg/entities/:id` via `api/kg.ts`, `api/kg-ops.ts`; agent inventory `/api/agents` |
| Storage | `http://127.0.0.1:18791/storage` | App tab `storage`, `StoragePage.tsx` | list/preview/download/upload/delete `/api/storage*` via `api/storage.ts`, `api/storage-ops.ts` |

Do not call embedding vendors, sync remote vault backends, upload production files, delete live objects, expand untrusted graph payloads into executable content, or invent S3/sandbox/cloud success. Test/sync/upload/delete remain disabled or DI-labeled unless the real local dependency is configured. Never add a live-looking action for a Dewee behavior that goso's cited APIs do not support.

## Constraints

- Reuse PageChrome / PageStatus / classifyPageState / inventoryBlocksMutation. Do not fork a second incompatible pattern.
- Error or permission must never render simultaneous zero-count empty claims or enabled mutation forms.
- Preserve last-known data only when clearly labeled stale with last successful refresh time.
- Untrusted memory/vault/kg bodies stay plain text, never HTML.
- Credential/secret invariant: never render, log, screenshot, fixture, or write to QA a token, private key, password, credential file contents, or raw vendor error that could contain one.
- Complete i18n in Vietnamese and English.
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Non-goals

- Copying Dewee pixels, wording, CSS, components, or source.
- Inventing embedding vendor, remote vault backend, paid search, canvas, or S3/cloud success.
- Knowledge Graph write/edit APIs.
- Merge and Vite restart (Codex CTO / advisor live QC).

## Acceptance criteria

1. Memory, Vault, Knowledge Graph, and Storage open with consistent page chrome, primary action, refresh, and filters where relevant.
2. Loading, true empty, filtered empty, generic error, permission, partial dependency failure, not-configured/DI, and stale are independently testable and never contradict one another.
3. Memory answers lane/provenance/index questions using existing APIs; create/edit/delete are gated and named.
4. Vault answers doc/link/sync/health/graph questions using existing APIs; agent/team inventory failure is not true-empty docs.
5. Knowledge Graph requires agent+scope, remains usable without canvas, and never treats inferred edges as facts.
6. Storage browse/upload/download/delete uses existing APIs; unconfigured/quota/truncated/leak states are honest; no S3/cloud success is claimed.
7. No embedding vendor, remote sync, paid search, or object-store success is claimed without factual configured evidence.
8. Vietnamese and English are complete.
9. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
10. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
11. Delivery is merged to `main` with `--no-ff` after SPEC 123 passes live QC, then only Vite `:3000` is restarted.

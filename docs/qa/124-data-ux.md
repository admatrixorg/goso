# QA — SPEC 124 DATA operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No live embedding vendors, remote vault backends, paid search, or object-store claims. No tokens, private keys, passwords, credential file contents, or vendor errors that could contain one in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Memory | `http://127.0.0.1:18791/memory` | Heading Memory; Refresh; Agent + Scope filters; Documents / Episodic Memory tabs; columns Path, Agent, Scope, Hash, Updated, Actions; document vs episodic lanes; All agents and All (global + personal) options. |
| Dewee Vault | `http://127.0.0.1:18791/vault` | Search documents; type chips (All, Context, Memory, Note, Skill, Episodic, Media, Document); All agents / All teams; bounded graph with zoom, fit, keyboard shortcuts, node-limit combobox. |
| Dewee Knowledge Graph | `http://127.0.0.1:18791/knowledge-graph` | Heading Knowledge Graph; “Select an agent” before graph load; agent combobox required. Also `/knowledge` 200 HTML. |
| Dewee Storage | `http://127.0.0.1:18791/storage` | Heading Storage; Upload + Refresh; delete folder/file; breadcrumbs; size on files (example `BOOTSTRAP.md 417 B`). |
| goso DATA tabs | App tabs `memory` `vault` `kg` `storage` | Nav present. Pages used `SectionHeader`, not SPEC 120 PageChrome/PageStatus. Unauth APIs 401. |

HTTP probes before edits: Dewee `/memory` `/vault` `/knowledge-graph` `/knowledge` `/storage` 200 HTML; goso `:3000/` and `/healthz` 200 `{"ok":true,...}`; goso `/api/memory` `/api/memory/index` `/api/vault/docs` `/api/vault/health` `/api/kg/graph` `/api/kg/index` `/api/storage` `/api/agents` `/api/teams` 401 `{"error":"unauthorized"}`. CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, Dewee `:18791` pid `11744`, Vite `:3000` pid `1885`, gateway `:18080` pid `68421` left running. No embedder call, no remote vault sync, no upload, no delete, no graph expand of untrusted HTML.

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` / `inventoryBlocksMutation` so loading / true-empty / filtered-empty / error / permission / dependency / not-configured / stale are exclusive. Error and permission never render a zero-count empty claim. Mutations stay closed while required inventory is error or permission. Independent failures keep provenance (notes vs agents/sessions; docs vs health/sync/graph; agents vs graph; listing vs preview).

| Page | Operator contract |
| --- | --- |
| Memory | First-class title. Save memory is the primary (form closed until opened). Refresh. Search + agent/session/lane filters. Episodic vs durable lanes. Index/FTS/embedding are configuration metadata, not vendor success. Agent/session inventory failure is a labeled dependency, not empty notes. Create/edit gated; delete confirms the named snippet/id. Bodies stay plain text. |
| Vault | First-class title. Put document is the primary (form closed until opened). Refresh + explicit Sync with progress and API-confirmed counts. Search + type/agent/team filters. Agent/team inventory failure falls back to document fields and is not true-empty docs. Health stale vs page stale vs health error are separate. Overwrite confirms the named title. Untrusted bodies stay plain. Sync does not claim a remote backend is healthy. |
| Knowledge Graph | First-class title. Refresh is the primary (read-only; no write API). Requires successful agent inventory plus explicit agent and scope. List and bounded graph (list) modes. Inferred/extracted labeled as not facts. Query-filtered empty distinct from true empty. Agent failure is not a true-empty graph. Leak handling remains; leak is not permission chrome. |
| Storage | First-class title. Upload is the primary. Refresh. Breadcrumbs, quota, truncated, and hidden/secret/runtime skip counts. `configured=false` is not-configured, not S3/cloud. Upload/delete disabled when listing is blocking, unconfigured, or quota-full. Typed path-named delete. Public-shape leak detection stays. |

i18n vi+en for all new operator copy (1839 keys match). GET listings still drop secret-shaped rows. Untrusted bodies remain React text in `<pre>`.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 251/251 pass (includes memory inventory-vs-list failure, filtered empty, embedding not-configured metadata, named delete target; vault permission-over-empty, agent/team fallback, health provenance, overwrite confirm; kg agent+scope, inferred-vs-posted, query-filtered empty; storage public-shape/quota/confirm, configured=false vs permission).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## DI-only gaps (honest unavailable, no fake live action)

- Dewee Memory document-file table (path/hash/mtime of markdown files). Goso memory is session notes + durable notes via `/api/memory*`.
- Dewee Vault canvas zoom/fit/node-limit 100–5000. Goso keeps a capped node/edge list with `vault.noCanvas`.
- Knowledge Graph write/edit APIs (none on goso). Canvas.
- Embedding vendor success, remote vault backend health, paid search, S3/cloud object-store connectivity.
- Storage browse of credential files, private keys, or internal runtime paths (public-shape helpers omit them).

## Out of scope

Merge and Vite `:3000` restart belong to Codex CTO / advisor live QC. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials or secret values are included in this record.

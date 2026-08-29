# QA — SPEC 101 Traces operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live vendor tokens.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Traces: search/filter paginated table, agent/channel/status, row detail/spans, tokens/latency/status | `docs/qa/090-goclaw-sidebar-ux.md` Traces |

goso mapping (self-written): live tab `traces` in [App.tsx](../../control-plane/src/App.tsx) still renders [TracesPage](../../control-plane/src/pages/TracesPage.tsx). Operator list is `GET /api/traces` with `items` (trace_id, ts, status, agent_id, channel when recorded, latency_ms, tokens, error), `total`/`offset`/`limit`, `error_groups`. Detail is `GET /api/traces/{id}` with redacted spans. Hash `#traces/{id}` is a stable link. Prompt/tool/result attributes are dropped server-side; secret-shaped attribute values become `[redacted]`. Tenant list/detail is scoped by `X-Goso-Tenant` when multi-tenant is on.

Out of scope: Logs page (111). Activity audit (110). Copying GoClaw dialogs. Live vendor tokens. SPEC 102.

## What changed

- List: search `q`, filters `agent`/`channel`/`status` when those fields exist, time range `from`/`to`, pagination `limit`/`offset`. Loading / empty / filter-empty / error. Error grouping chips.
- Detail: spans tree, token/latency/status summary. Bounded public tree (`MaxPublicSpans`). Hash `#traces/{id}`. Partial-data empty when spans are missing.
- Redaction: drop prompt/tool_input/tool_result/arguments/result; redact token-shaped errors and secret attribute keys. Client `publicAttrs` / `publicHasSecrets` as a second gate.
- Recording: agent span gets `agent_id` and `tenant_id` from the session. LLM spans record input/output tokens.
- Tenant isolation: list and detail hide other tenants. Default tenant when multi-tenant is off.
- i18n vi+en. CP typecheck. Helper tests + observe HTTP. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/observe ./gateway/internal/pipeline ./gateway/internal/agent -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `parseTraceHash` / `tracesHash`, `rangeFrom`, `statusOf`, `tokenTotal`, `uniqueValues`, `groupErrors`, `publicAttrs` dropping prompt/api_key, `publicHasSecrets`, `capText`, `pageLabel`.
- `go test` observe: list filters (agent/channel/status/q/from), pagination + truncated, detail redacts prompt/tool/result/sk- keys, `tenant_id` is not treated as a secret key, 404 missing and cross-tenant, `/v1/traces/{id}` alias. Existing ring/span-tree/wrap tests still pass.
- Pipeline/agent chat still records nested agent/llm/tool spans on `GET /api/traces`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Logs (111). Activity audit (110). Copying GoClaw dialogs. Live vendor tokens. Binding/killing demo ports. Merge. SPEC 102.

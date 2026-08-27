# QA — SPEC 052 Gateway `/v1/*` aliases for existing `/api/*`

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

MCP clients still call GoClaw-shaped `/v1/providers` `/v1/agents` `/v1/sessions` `/v1/memory` `/v1/teams` `/v1/channels` `/v1/traces` `/v1/skills` `/v1/webhooks`. Gateway previously only had `/api/*`.

## What changed

- Same mux, same handler: GET lists that already exist under `/api` are also served at `/v1`.
  - GET `/v1/providers` `/v1/agents` `/v1/sessions` `/v1/channels` `/v1/traces` `/v1/skills` `/v1/webhooks` `/v1/teams` `/v1/memory`
  - GET `/v1/memory` still requires `session_id` (same 400 as `/api/memory`).
  - POST `/v1/chat` uses the same handler as POST `/api/chat`.
- Auth is unchanged: empty token still 401 on `/v1/*`. View token GET is allowed on `/v1/agents` and `/v1/sessions` the same way as `/api`. Body cap (1 MiB) also applies to `/v1`.
- No invented CRUD: GET `/v1/agents/{id}` and GET `/v1/cron` stay 404. MCP `http-client.ts` has a base-path comment only; tool paths were not rewritten.

## Commands

```
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- httptest GET `/v1/providers` 200, JSON identical to GET `/api/providers` (`TestProvidersListsConfigured`, `TestV1AliasesMatchAPI`).
- httptest GET lists for agents, sessions, channels, skills, webhooks, teams, memory match `/api` (`TestV1AliasesMatchAPI`). GET `/v1/traces` matches `/api/traces` (`TestHandleTraces_SpanTrees`).
- POST `/v1/chat` echoes through the existing chat handler (`TestV1ChatSameHandler`).
- Auth: empty token GET `/v1/providers` 401; view token GET `/v1/agents` `/v1/sessions` 200, POST `/v1/chat` 403.

## Non-goals

Fake CRUD that `/api` does not have, rewriting 66 MCP tools, live demo bind/kill, copying goclaw/ZaloCRM.

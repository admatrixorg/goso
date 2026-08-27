# QA — SPEC 043 Control Plane UI (035–042 APIs)

Date: 2026-08-27. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Secrets (provider keys, channel tokens, webhook token/hmac) are never listed; webhook secret is shown **once** on create then redacted after copy.

## Wired endpoints

Live sidebar tabs (not DEMO-only) in `control-plane/src/App.tsx`: Teams, Vault, Memory, Providers, Channels, Webhooks, Traces. DEMO tabs unchanged.

| Page | Client | Methods wired |
|------|--------|----------------|
| Teams | `src/api/teams.ts` | `GET/POST /api/teams` `GET/PUT /api/teams/{id}` `GET/POST /api/teams/{id}/members` `GET/POST /api/teams/{id}/tasks` `PATCH /api/teams/{id}/tasks/{tid}` `GET/POST /api/teams/{id}/messages` `GET/POST /api/agents/{id}/links` `GET /api/agents/{id}/evolution` `POST /api/agents/{id}/evolution/{sid}/apply` |
| Vault | `src/api/vault.ts` | `GET /api/vault/docs` `GET /api/vault/docs/{id}` `PUT /api/vault/docs` `GET /api/vault/docs/{id}/links` `GET /api/vault/search?q=` `POST /api/vault/sync` |
| Memory | `src/api/memory.ts` | `GET /api/sessions` (picker via `client.ts`) `GET /api/memory?session_id=` `POST /api/memory` `GET /api/memory/search?q=` |
| Providers | `src/api/providers.ts` | `GET /api/providers` → `{providers: string[]}` names only |
| Channels | `src/api/channels.ts` | `GET /api/channels` → `{channels: [{name, configured}]}` boolean only |
| Webhooks | `src/api/webhooks.ts` | **`POST /api/webhooks` only**. Last create `{id, token, token_prefix, hmac_key}` in session memory; token/hmac hidden after copy. |
| Traces | `src/api/traces.ts` | `GET /api/traces` → `{traces, spans}` rendered defensively |

All new clients use `jsonFetch` + Bearer from `src/api/client.ts` (`VITE_GOSO_ADMIN_TOKEN` / `localStorage.goso_token`). i18n keys in both `vi.ts` and `en.ts`.

`api.channels` in `client.ts` was corrected from `string[]` to `{name, configured}[]` to match the gateway catalog.

## Missing

| Endpoint | Status |
|----------|--------|
| **`GET /api/webhooks`** | **Does not exist.** UI does **not** invent a registry list. Create button + last-created id/prefix only. `POST /api/webhooks/llm` exists on the gateway but is not a CP list surface. |

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD <sibling>/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Coordinator after merge: restart `:18080` with new `goso-gateway`; Vite `:3000` proxies to `:18080`.

## Non-goals

New Go endpoints, CRM UI, killing CRM demo, GET webhook registry.

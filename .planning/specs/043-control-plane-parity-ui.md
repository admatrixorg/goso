# SPEC 043 — Control Plane UI for 035–042 + refresh demo gateway

> LOCKED: 2026-08-27. Clean-room React in `control-plane/`. **No** copy from ZaloCRM or goclaw-source. No banned author ids. Demos **`:8082` / `:8091` must not be killed.** Coordinator restarts `:18080` with **new** `goso-gateway` from this main and Vite `:3000` after merge.

User: “sao UI vẫn vậy” — (a) `:18080` binary 2026-08-20 (b) 035–042 are Go APIs (c) CP has no Teams/Vault/Memory/Providers/Channels/Webhooks/Traces pages.

## Existing APIs on `origin/main` (`cabc794`+) — **only wire these**

| Page | Methods that exist |
|------|-------------------|
| Teams | `GET/POST /api/teams` `GET/PUT /api/teams/{id}` `GET/POST /api/teams/{id}/members` `GET/POST /api/teams/{id}/tasks` `GET/POST /api/teams/{id}/messages` `GET/POST /api/agents/{id}/links` `GET /api/agents/{id}/evolution` `POST /api/agents/{id}/evolution/{sid}/apply` |
| Vault | `GET /api/vault/docs` `GET /api/vault/docs/{id}` `PUT /api/vault/docs` `GET /api/vault/docs/{id}/links` `GET /api/vault/search?q=` `POST /api/vault/sync` |
| Memory | `GET /api/memory?session_id=` `POST /api/memory` `GET /api/memory/search?q=` |
| Providers | `GET /api/providers` → `{providers: string[]}` — **never show secrets** |
| Channels | `GET /api/channels` → `{channels: [{name, configured}]}` |
| Webhooks | **`POST /api/webhooks`** (create, secret **once**). **`POST /api/webhooks/llm`**. **No `GET /api/webhooks` list** — do **not** invent it. UI: create button + last created id/prefix (redact token after first paint). |
| Traces | `GET /api/traces` (observe) |

Missing (report only): GET webhook registry list.

## UI (match existing ZAgent pages)

New pages under `control-plane/src/pages/` + `src/api/*.ts` using the same `jsonFetch`/Bearer pattern as `api/client.ts`. Add tabs to **live** sidebar (not DEMO-only): Teams, Vault, Memory, Providers, Channels, Webhooks, Traces.

Reuse `Button`, `Card`, `EmptyState`, `SectionHeader`, `Icon` (`list`, `doc`, `layers`, `bolt`, `hook`, `inbox`, `history`, `search`, `plus`, `refresh`). Add i18n keys to **both** `vi.ts` and `en.ts` (`MsgKey`).

Behavior:

- Teams: list teams; create; pick team → members, Kanban columns from tasks `todo|doing|done`, mailbox messages. Show agent links + evolution list (apply if POST exists).
- Vault: list docs; search box → `/api/vault/search`; put title+body; show links inbound/outbound; sync button.
- Memory: session picker (`GET /api/sessions`) then `GET /api/memory?session_id=`; search q; POST note.
- Providers: table of names from API; empty = echo only. No token fields.
- Channels: 7 rows name + configured yes/no (boolean only).
- Webhooks: POST create; display returned fields except full secret after copy; note “no list API”.
- Traces: table/tree from `GET /api/traces` (render JSON spans defensively).

`npm run typecheck` in `control-plane/` must pass. Keep DEMO tabs behavior.

## Worker

Do **not** bind/kill 8082, 8091, 3000, 18080, 18088. Commit on branch. `docs/qa/043-control-plane-ui.md` lists wired vs missing endpoints.

## QC

- `cd control-plane && npm run typecheck`
- `go test ./...` + `go build ./gateway/cmd/goso-gateway` (must stay green)
- sibling `goso-crm/scripts/agpl-check.sh` 0
- Coordinator after merge: restart `:18080` new binary; Vite `:3000` proxy to `:18080`; curl index 200 + 2–3 new APIs 200.

## Non-goals

New Go endpoints (except if a one-line GET webhooks is *necessary* — prefer document missing). CRM UI. Killing CRM demo.

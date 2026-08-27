# SPEC 052 — Gateway `/v1/*` aliases for existing `/api/*`

> LOCKED: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

MCP clients still call GoClaw-shaped `/v1/providers` `/v1/agents` `/v1/sessions` `/v1/memory` `/v1/teams` `/v1/channels` `/v1/traces` `/v1/skills` `/v1/webhooks`. Gateway only has `/api/*`.

## Do

Register **aliases** on the same mux: each existing method+path under `/api/...` also served at `/v1/...` for the **same handler** (no second implementation). Minimum GET lists that exist today:

- GET `/v1/providers` `/v1/agents` `/v1/sessions` `/v1/channels` `/v1/traces` `/v1/skills` `/v1/webhooks` `/v1/teams` `/v1/memory` (memory still needs `session_id` query like `/api/memory`)
- POST `/v1/chat` same as `/api/chat` if cheap.

Do **not** invent missing CRUD (no fake GET if `/api` does not have it). Tests: httptest GET `/v1/providers` 200 same JSON as `/api/providers`. Auth still required.

Optional: MCP `http-client.ts` base path comment. Do not rewrite all 66 tools.

`docs/qa/052-v1-aliases.md`. Commit `admatrixmdp/spec052-v1-aliases`. Do not merge.

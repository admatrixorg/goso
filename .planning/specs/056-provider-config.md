# SPEC 056 — Providers page: configure LLM connections

> LOCKED: 2026-08-28. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> **Do not break** env `router9` + default `ocg/deepseek-v4-flash` (demo live chat).

User: UI only lists names; need full **configure** (BaseURL, model, key, test).

Today on main: **only** `GET /api/providers` `{providers: string[]}`. No POST/PATCH/test. Secrets package `gateway/internal/secrets` AES-256-GCM exists (`GOSO_MASTER_KEY`).

## HTTP — add these (do not invent extra)

| Method | Path | Body / response |
|--------|------|-----------------|
| GET | `/api/providers` | `{providers:[{name, type, base_url, model, key_set, source}]}` — **never** `api_key`. Keep a `names` array of strings **also** for old tests *or* update tests. `key_set` boolean. `source` = `env` \| `sqlite`. |
| POST | `/api/providers` | `{name, type, base_url?, model?, api_key?}`. Types: `openai-compat` \| `anthropic` \| `echo` \| `router9`. 201. Secret stored via `secrets` (`provider:{name}:api_key`); if no master key and api_key set → 400 `master key required` (env overlay still works). Empty api_key = no secret row. |
| PATCH | `/api/providers/{name}` | `{type?, base_url?, model?, api_key?}`. Empty `api_key` = **unchanged**. Never returns secret. 404 if missing. |
| POST | `/api/providers/{name}/test` | Optional `{kind:"models"\|"chat"}`. **Real HTTP** to that provider (models list or 1-turn chat). Response `{ok, latency_ms, models?, reply?, error?}`. **Do not fake ok.** Timeout ≤ 20s. 404 if unknown name. |

Env overlay: `NewRegistry()` env providers remain. SQLite rows merge; **env wins** on name clash so demo router9 stays. Reload registry after POST/PATCH (or lookup sqlite on Get).

Tests: httptest fake OpenAI `/v1/models` 200; POST provider; GET `key_set true` no secret; PATCH empty key unchanged; TEST returns ok + model ids; fake 401 → `{ok:false, error:…}`. Empty env GET still includes `echo` (+ `router9` if URL set).

## UI

`ProvidersPage` (not names-only): table name/type/base_url/model/`key_set`; add/edit form; password key field never echoed; Test button shows **raw gateway test JSON** (error included). StatusLine loading/error. i18n vi+en.

`docs/qa/056-provider-config.md`. Commit `admatrixmdp/spec056-provider-config`. Do not merge.

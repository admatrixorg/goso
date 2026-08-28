# QA — SPEC 056 Providers page: configure LLM connections

Date: 2026-08-28. Clean-room. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Today on main was names-only `GET /api/providers`. This spec adds full configure (base URL, model, key, test) with SQLite persist and env overlay.

## What changed

- `GET /api/providers` returns `{providers:[{name,type,base_url,model,key_set,source}]}`. Never `api_key`. `source` is `env` or `sqlite`. Tests updated from the old string array.
- `POST /api/providers` `{name,type,base_url?,model?,api_key?}`. Types: `openai-compat` | `anthropic` | `echo` | `router9`. 201. Secret stored via `gateway/internal/secrets` as `provider:{name}:api_key`. Empty api_key = no secret row. No master key + api_key set → 400 `master key required`.
- `PATCH /api/providers/{name}` `{type?,base_url?,model?,api_key?}`. Empty `api_key` = unchanged. 404 if missing. Env-sourced names are not overwritten (`env overlay`).
- `POST /api/providers/{name}/test` optional `{kind:"models"|"chat"}`. Real HTTP (models list or 1-turn chat). Response `{ok,latency_ms,models?,reply?,error?}`. Never fake ok. Timeout 20s. 404 if unknown name.
- SQLite `llm_providers(name,type,base_url,model)` plus secrets table. Env registry from `NewRegistry()` merges; **env wins** on name clash so demo router9 stays.
- Control-plane `ProvidersPage`: table name/type/base_url/model/key_set/source; add/edit form; password key field never echoed; Test shows raw gateway JSON; StatusLine loading/error; i18n vi+en.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Empty env GET includes `echo`; router9 appears when `GOSO_ROUTER9_BASE_URL` is set (`TestProviders_EmptyEnvIncludesEcho`, `TestProviders_Router9WhenURLSet`, `TestProvidersListsConfigured`).
- POST + GET `key_set true` and no secret in body; PATCH empty key leaves ciphertext; TEST httptest `/v1/models` 200 returns model ids; fake 401 → `{ok:false,error:…}` (`TestProviders_CRUDTestConnection`).
- No master key + api_key → 400 (`TestProviders_NoMasterKeyWithAPIKey400`).
- PATCH/TEST unknown name 404 (`TestProviders_PatchMissing404`).
- SQLite row named `router9` does not replace env overlay (`TestProviders_EnvWinsOverSQLite`).
- Memory + SQLite store CRUD/persist (`TestStore_LLMProviderCRUD`, `TestSQLiteStore_LLMProviderPersist`).
- Probe unit: httptest models 200/401 (`TestProbe_ModelsOKAnd401`).

## Non-goals

MCP `/v1/providers` extra CRUD rewrite, DELETE provider, live paid API calls, product keys in git, binding/killing demo ports, copying goclaw/ZaloCRM.

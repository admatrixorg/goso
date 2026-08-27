# QA — SPEC 045 router9 default `ocg/deepseek-v4-flash` + unique IDs

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No product secrets in git. Unit tests do **not** call live `127.0.0.1:20127`.

## Catalog default

| Surface | Status |
|---------|--------|
| `OpenAICompatProviders` `router9.Model` | **`ocg/deepseek-v4-flash`** (was `cx/gpt-5.6-sol`). |
| `GOSO_ROUTER9_MODEL` | Still overrides catalog. `cx/*` remains constructable (tested with `cx/gpt-5.6-sol`). |
| Construct | Unchanged: `GOSO_ROUTER9_BASE_URL` non-empty; API key may be empty. |
| SETUP / `.env.example` / 044 env sample | Default flash. |

`cx/*` is out of the **default** test loop (Codex login no longer required to smoke chat). It is not deleted.

## Unique IDs

`store.newID()` was `YYYYMMDD-` + in-process `sqliteSeq`. Seq reset to 0 on process start → `20260827-1` collided with existing agents/sessions/messages (`UNIQUE constraint failed`) → chat 502 after gateway restart.

Fix: `YYYYMMDD-` + 16 hex chars from `crypto/rand` (8 bytes). Fallback if `rand.Read` fails: process seq + unix nano. Shared across agents/sessions/messages/teams/docs.

Unit test `TestSQLiteStore_RestartSafeIDs`: two `OpenSQLite` opens on the same file; after close + `sqliteSeq=0`, second `CreateAgent` / `CreateSession` / `AddMessage` succeed.

## Live smoke (coordinator-owned)

Coordinator already ran live pipeline smoke **with env** `GOSO_ROUTER9_MODEL=ocg/deepseek-v4-flash` (before this SPEC made it the catalog default):

| Probe | Result |
|-------|--------|
| Gateway `POST /api/chat` | **HTTP 200** `{"reply":"OK"}` `model=ocg/deepseek-v4-flash` `provider=router9` latency ~1634ms |

Do **not** invent a different reply. This SPEC does not re-run live 9Router from unit tests. Coordinator restarts demo after merge.

044 live `cx/gpt-5.6-sol` 401 `token_expired` remains historical (not a product success). Drop Codex login from the default test loop.

## Commands

```
go test ./...
go build -o /tmp/goso-gateway-045-qc ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD <sibling>/goso-crm/scripts/agpl-check.sh
```

Control-plane typecheck skipped (no CP files in this SPEC). Do not bind or kill demo ports.

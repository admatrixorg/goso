# SPEC 045 — router9 default `ocg/deepseek-v4-flash` + unique IDs

> LOCKED: 2026-08-27. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids.
> Worker must **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`. Coordinator restarts demo after merge.

User verified: 9Router `ocg/deepseek-v4-flash` chat **HTTP 200, no auth**, reply `OK`. **Drop Codex login** from the test loop (`cx/*` out of default).

Coordinator already ran live pipeline smoke with **env** `GOSO_ROUTER9_MODEL=ocg/deepseek-v4-flash` → `POST /api/chat` **200** `{"reply":"OK"}` `model=ocg/deepseek-v4-flash` `provider=router9` latency ~1634ms. This SPEC makes that the **catalog default** and fixes the ID bug that 502'd chat after gateway restart.

## 1. Default model

In `gateway/internal/llm/compat.go` router9 catalog:

- `Model: "ocg/deepseek-v4-flash"` (was `cx/gpt-5.6-sol`)
- Env `GOSO_ROUTER9_MODEL` still overrides
- Tests that assert `cx/gpt-5.6-sol` must assert the new default
- `docs/SETUP.md`, `.env.example`, `docs/qa/044-router9-functions.md` env sample: default flash
- `docs/qa/045-router9-deepseek.md` records coordinator live smoke (do **not** invent a different reply)

Keep `cx/*` constructable via `GOSO_ROUTER9_MODEL` — not deleted.

## 2. Unique IDs (blocks demo after restart)

`store.newID()` is `YYYYMMDD-` + in-process `sqliteSeq`. Seq resets to 0 on process start → `20260827-1` collides with existing agents/sessions/messages (`UNIQUE constraint failed: messages.id` / `sessions.id` → chat 502).

Replace with restart-safe unique IDs (random hex suffix and/or persisted sequence). Must not collide across agents/sessions/messages/teams/docs. Tests: create rows, simulate seq reset (or just two Store instances on the same file) → second CreateAgent/CreateSession/AddMessage succeeds.

## 3. QC

`go test ./...`, `go build ./gateway/cmd/goso-gateway`, `cd control-plane && npm run typecheck` if CP files change, sibling `goso-crm/scripts/agpl-check.sh` 0.

Commit on `admatrixmdp/spec045-router9-deepseek`. **Do not merge. Do not kill demo ports.**

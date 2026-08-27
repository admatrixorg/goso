# SPEC 036 — Memory L0/L1 + FTS5 (no pgvector)

> LOCKED: 2026-08-27. Clean-room. Closes matrix **M1, M2, M5** (L0/L1 + MCP-targetable `/api/memory`). **M3/M4 L2 + pgvector = DI-09 — do not implement.** D5 FTS5 on Lite SQLite: yes.
> Depends on SPEC 035 (memory/summarize stages exist as hooks). If 035 is already on this branch’s base (`origin/main`), plug into those stages.
> Demos `:8082` `:8091` `:3000` `:18080` `:18088` — do not kill or bind. No goclaw copy. No banned author ids.

## Goal

Working memory (L0) stays session messages. Add **episodic L1**: after a run (or when history exceeds a threshold), write a short **session summary** row and use it on the next **history/prompt** stage. Add **FTS5** over summaries + recent messages for Lite/SQLite search. HTTP **`/api/memory`** so the existing MCP client can be pointed later (today MCP talks `/v1/memory` — add **goso `/api/memory`**; optional alias note in QA, do not invent `/v1` unless cheap).

## L0

Session `messages` already exist (`store`). Keep them as working memory. History cap remains SPEC 035 (50 turns). Optional: `POST /api/memory/flush` copies last user+assistant into a memory note — not required if L1 summarize covers it.

## L1 episodic

- Table (SQLite): `session_summaries` (`session_id`, `summary` text, `updated_at`) or `memories` (`id`, `session_id`, `kind='episodic'`, `body`, `created_at`).
- Fill from pipeline **summarize** stage: if message count ≥ **12** or caller sets `summarize=1`, produce a ≤ **500 rune** summary. Prefer LLM if provider is not Echo; Echo: first+last user lines concatenated (deterministic, tested).
- Next run **history** stage: prepend summary as a system or `role=system` note “Previous summary: …” without sending the dropped middle turns if over cap.
- Tests: 12+ Echo turns → summary row exists; next Chat includes summary text in the provider-recorded prompt.

## FTS5 (Lite)

- SQLite virtual table FTS5 over memory body (summaries + optional message content). `modernc.org/sqlite` already in go.mod — use FTS5; skip if compile tag missing (must work in `go test`).
- `GET /api/memory/search?q=` returns ranked rows `{id, session_id, kind, snippet}` Bearer-auth like other `/api/*`. Empty q → 400. No results → `[]`.
- In-memory store (`store.Store`): simple substring search so tests without SQLite still pass.

## HTTP (MCP-targetable)

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/api/memory?session_id=` | list episodic items for session |
| POST | `/api/memory` | `{session_id, body, kind?}` kind default episodic |
| GET | `/api/memory/search?q=` | FTS5 / substring |

401 without Bearer when auth on. No UID/phone in bodies. Do **not** add `/v1/memory` unless it is a one-line alias; document MCP still pointed at `/v1` as PARTIAL until a later MCP SPEC.

## Pipeline plug

035 memory/summarize no-ops: replace with real functions. Do not rewrite the 8-stage order.

## Non-goals

pgvector, knowledge graph (M3/M4), vault wikilinks (037), embeddings API, changing CRM eventstore.

## QC

`go test ./...`, `make build` or `go build ./gateway/cmd/goso-gateway`, sibling `goso-crm/scripts/agpl-check.sh` 0, `docs/qa/036-memory.md`. Commit, do not merge, do not restart demos.

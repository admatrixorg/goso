# QA — SPEC 036 Memory L0/L1 + FTS5 (no pgvector)

Date: 2026-08-27. Clean-room. Closes matrix rows **M1, M2, M5**. **M3/M4 L2 + pgvector = DI-09 — not implemented.** D5 FTS5 on Lite SQLite: yes.

## What changed

L0 working memory remains session `messages` (history cap 50 from SPEC 035).

L1 episodic summaries live in `memories` (`kind=episodic`) on both the in-memory store and SQLite. The pipeline **summarize** stage writes a ≤500 rune summary when the session has ≥12 messages or the caller sets `summarize=1` (JSON `true`/`1` or query `?summarize=1`). Echo (and the test `scripted` provider): first + last user lines, deterministic. Other providers: one extra LLM chat, then truncate; fallback to Echo summary on failure. The pipeline **memory** stage is plugged (L0 is already persisted messages). Stage order is unchanged: context → history → prompt → loop(think → act → observe) → memory → summarize.

Next **history** stage prepends a system note `Previous summary: …` from the latest episodic row, then caps/sanitizes as in 035 (dropped middle turns are not resent).

FTS5 (`modernc.org/sqlite`) indexes memory bodies and message content. If the virtual table cannot be created, search falls back to `instr()`. In-memory store: case-insensitive substring. `GET /api/memory/search?q=` returns ranked rows `{id, session_id, kind, snippet}`. Empty `q` → **400**. No hits → `[]`.

HTTP (Bearer like other `/api/*`):

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/api/memory?session_id=` | list memory rows for the session (`{"memories":[…]}`) |
| POST | `/api/memory` | `{session_id, body, kind?}` kind default `episodic` |
| GET | `/api/memory/search?q=` | FTS5 / substring |

401 without Bearer when auth is on. No UID/phone fields. `/v1/memory` was **not** added. MCP client still points at `/v1/memory` — **PARTIAL** until a later MCP SPEC retargets `/api/memory`.

## Commands

```
go test ./...
gofmt -l gateway
go vet ./gateway/...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
/Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- 12 Echo chats → episodic summary row; next Scripted chat records `Previous summary:` in the provider prompt (`TestChat_EchoTurnsWriteSummaryAndNextPrompt`).
- `summarize=1` with one turn still writes a summary (`TestChat_SummarizeFlag`, `TestMemoryAPI_SummarizeFlag`).
- In-memory substring + SQLite FTS/instr search (`TestStore_MemoryAndSearch`, `TestSQLiteStore_FTSAndSummary`).
- HTTP list/create/search + empty `[]` + 400 empty q + Bearer 401 (`TestMemoryAPI_*`).

## Non-goals

pgvector / knowledge graph (M3/M4), vault wikilinks (037), embeddings API, CRM eventstore, `/v1/memory` alias.

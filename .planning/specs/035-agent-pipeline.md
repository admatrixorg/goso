# SPEC 035 — Agent pipeline + 4-mode prompt

> LOCKED: 2026-08-27. Clean-room Go. Closes matrix rows **P1–P4, Q1–Q4, K0 (loop side)**.
> Read behavior from GoClaw README / `docs/01-agent-loop.md` — **do not copy** `goclaw-source` code, names of unexported goclaw types, or file layout. No `minhhaiphan`/`locphamnguyen` outside `.planning`.
> Demos `:8082` `:8091` `:3000` `:18080` `:18088` — do not kill or bind.

## Goal

Replace `Runtime.Chat` keyword `matchTools` with a **self-written 8-stage runner** that iterates **think → act → observe** until the LLM returns final text (max 20). Advertise connector tools to the LLM; execute only **tool_use** the model requested. Four prompt modes Full / Task / Minimal / None. In-process lifecycle hooks. History sanitize (orphan tool turns, turn cap).

`POST /api/chat` keep working. Echo without tools still returns `echo: …`.

## Stages (README names — implement these, not a goclaw package dump)

Order **once per run** then loop:

1. **context** — load session + agent; put agent_id / session_id / mode on a run state (no per-user workspace files yet — 036/041).
2. **history** — `ListMessages`; cap last **N=50** turns; **sanitize**: drop `tool` rows that have no preceding assistant tool_use; drop unmatched tool_use.
3. **prompt** — build system prompt from mode (Q1). Attach as `system` message. Do **not** implement Anthropic `cache_control` (039).
4. **think** — call LLM with messages + advertised tools. Record one observe trace if Observer is present (optional wrap).
5. **act** — if the model returned tool_use, `CallTool` (existing approval gate). Else break loop (final text).
6. **observe** — append assistant tool_use + tool result messages; continue loop.
7. **memory** — **no-op hook** in 035 (036 fills). Must be a named stage so 036 can plug in without rewriting the runner.
8. **summarize** — **no-op hook** in 035 (036 fills). Called after loop (and may be called from history later).

Loop body is stages 4–6, max **20**. If cap hit: return last assistant text or a short `max_iterations` error string (documented in QA). Pending approval: stop iterating; keep current `pending_approval` behavior in `ChatResult.Trace`.

## LLM tool_use (replace matchTools)

Today `llm.Provider` is `Chat(ctx, []Message) (string, error)`.

Add an **optional** interface (name it in goso, not goclaw), e.g. `ToolChat`:

```
ChatTools(ctx, messages []Message, tools []ToolSpec) (Reply, error)
Reply = { Text string, ToolCalls []ToolCall }
ToolCall = { ID, Name, Connector?, Arguments map[string]any }
```

- If provider implements it, pipeline uses it.
- If not: `Chat` as now (text only) — **no keyword tool dispatch**.
- Echo: text only.
- Tests: a **scripted** fake provider (test file) that on turn 1 returns one `contact_search` tool_use, on turn 2 returns final text. Production Anthropic/OpenAI in 035 may stay text-only (039 adds SSE/native tools). That is OK: the **loop** must still execute ToolCalls when a ToolChat provider returns them.

**Delete** `matchTools` / `intentMatch` / `extractArgs` from production `runtime.go`.

Tool names advertised to the LLM: `connector__tool` (double underscore) or `connector.tool` — pick one, document, parse on act. Connector tools still go through `Runtime.CallTool`.

## 4-mode prompt (Q1–Q4)

Modes: `full` | `task` | `minimal` | `none` (default **`full`**).

Resolution order: `POST /api/chat` JSON `prompt_mode` → session label/metadata if you add a field → default full. Persist mode on the session if you add a column; otherwise request-only is OK for 035 (document).

Section gating (clean-room copy, not goclaw 15+ section dump):

| Mode | System prompt |
|------|----------------|
| full | identity + instructions + tool usage notes + safety one-liner |
| task | instructions + tool usage notes |
| minimal | one-line instruction |
| none | empty system (user/history only) |

Q2 cache-boundary: **out of 035** (039). Q3: request/session resolution as above. Q4 bootstrap AGENTS.md files: **stub** — if files are absent, skip (036/038). Do not invent a goclaw context-file tree.

## Hooks (P4)

In-process dispatcher (no plugins on disk in 035):

`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`

Each hook: `func(ctx, HookEvent)`. Default empty. Tests register a recorder. Failures in hooks must not abort the run (log and continue) except PreToolUse returning a documented skip/deny is optional — if implemented, test it.

## HTTP

`POST /api/chat` body may include `"prompt_mode":"task"`. Unknown mode → 400. Existing `{session_id, message}` unchanged.

## Tests (must)

- Scripted LLM: user “search A” → think returns tool_use → act invokes fake connector → observe → think returns text. `role=tool` message stored. `matchTools` not used.
- Echo: no tools, reply `echo: …`, no tool messages.
- History sanitize: inject orphan tool row; it is not sent to LLM.
- Prompt mode none: no system message (or empty) in the payload the fake provider recorded.
- Prompt mode full: system message non-empty.
- Max 20: scripted provider always returns tool_use → stop, no infinite loop.
- Hooks: PreToolUse + PostToolUse + Stop fired on the tool path.
- Existing connector ListTools / CallTool tests stay green.
- **Update** `TestTools_InvokeStoresRoleToolAndTrace` (currently `"tìm khách A"` + Echo) to the scripted provider.
- **Update** `scripts/e2e-connector.sh` chat step so it does **not** rely on keyword match. Prefer a test-only scripted provider behind env (`GOSO_E2E_SCRIPTED=1`) used only in that script, or assert tools via `/api/tools/invoke` and chat as text. Do not bind 8082/8091/3000/18080 (e2e already uses ephemeral ports — keep that).

## Files (suggested, not mandatory layout)

New: `gateway/internal/pipeline/` (runner + stages + hooks + modes). Wire from `agent.Runtime.Chat`. LLM types in `gateway/internal/llm`. Header: `Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.`

## QA

`docs/qa/035-agent-pipeline.md`: commands + how to prove matchTools is gone (`git grep matchTools` empty in `gateway/`).

## QC

- `go test ./...`
- `go build -o bin/goso-gateway ./gateway/cmd/goso-gateway` (or `make build`)
- `gofmt` / `go vet`
- Sibling `/Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` exit 0 (no new author ids; no goclaw paste)
- No Node in gateway
- Commit on this branch; do not merge; do not restart demos

## Non-goals

pgvector, vault, teams, SSE/prompt cache, Discord/WhatsApp, SSRF/injection layers (041), OTLP (042), copying goclaw `internal/pipeline`.

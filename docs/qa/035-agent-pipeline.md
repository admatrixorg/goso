# QA — SPEC 035 Agent pipeline + 4-mode prompt

Date: 2026-08-27. Clean-room 8-stage runner. Closes matrix rows **P1–P4, Q1–Q4, K0 (loop side)**.

## What changed

`Runtime.Chat` is an 8-stage runner: context → history → prompt → loop(think → act → observe, max 20) → memory (no-op) → summarize (no-op).

Tools run only from LLM `ToolCalls` (`llm.ToolChat`). Echo stays text-only. Keyword intent dispatch is gone from production.

Prompt modes: `full` | `task` | `minimal` | `none` (default `full`). `POST /api/chat` accepts `prompt_mode`. Unknown → **400**. Request-only in 035 (not persisted on the session).

Tool names advertised as `connector__tool`. `connector.tool` is also parsed on act.

History: cap last **50** messages, then sanitize (orphan tool rows / unmatched tool_use). Cap-then-sanitize so a window that starts on a tool row does not send an unmatched result to the LLM.

Hooks (in-process): `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`. Hook panics are logged and do not abort the run.

If the loop hits 20 iterations without final text, the reply is `max_iterations`.

Pending approval still stops the loop and appears on `ChatResult.Trace`.

Q2 cache-boundary and Q4 bootstrap file tree are out of 035. `AGENTS.md` in process cwd is appended to full/task when present; missing files are skipped.

## Commands

```
go test ./...
gofmt -l gateway
go vet ./gateway/...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
git grep matchTools -- gateway
# expected: empty
/Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Optional e2e (ephemeral ports only — does not bind 8082/8091/3000/18080/18088):

```
GOSO_E2E_SCRIPTED=1  # set inside scripts/e2e-connector.sh
sh scripts/e2e-connector.sh
```

`GOSO_E2E_SCRIPTED=1` plus `GOSO_ENV=test` selects a test-only `llm.Scripted` ToolChat provider (turn 1 = `contact_search` tool_use, turn 2 = text). Either flag alone is ignored. Do not set in production.

## Proof keyword dispatch is gone

`git grep matchTools -- gateway` must print nothing. Echo + user text that used to be an intent phrase does not create `role=tool` messages (`TestChat_EchoNoTools`).

## Non-goals (still later specs)

pgvector/vault/teams, SSE/prompt cache (039), Discord/WhatsApp, SSRF/injection (041), OTLP (042).

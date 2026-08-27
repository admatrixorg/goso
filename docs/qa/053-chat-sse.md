# QA — SPEC 053 Chat SSE in control-plane

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

D3 PARTIAL was JSON-only `POST /api/chat`. Gateway OpenAI client already parsed upstream SSE when `stream=true` (039). This spec streams **the control-plane chat HTTP response**.

## What changed

- Default `POST /api/chat` (and aliased `POST /v1/chat`) stays JSON `{reply, session_id, …}`.
- If `Accept: text/event-stream` **or** JSON body `"stream": true`: `Content-Type: text/event-stream` with two `data: {"delta":"…"}` frames then `data: [DONE]`. Same auth / body cap as before.
- Validation, missing session, quota, injection stay JSON 4xx/5xx **before** the stream. Provider failure after a stream is requested is one `event: error` line (`data: {"error":"…"}`). JSON POST without stream still 502 on provider failure.
- ChatPage prefers fetch SSE (`Accept` + `stream: true`), appends deltas to a local assistant bubble, then reloads persisted messages. `StatusLine` loading/error and the send-fail assistant bubble from 046 stay. Fallback: HTTP 406, missing `ReadableStream`, or a non-`text/event-stream` success body → JSON POST (`api.chat`).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Echo via `Accept: text/event-stream`: two delta frames then `[DONE]`, concat `echo: hi there` (`TestChatSSE_EchoAcceptTwoFramesThenDONE`).
- Echo via body `"stream": true` without Accept (`TestChatSSE_EchoBodyStreamTrue`).
- JSON POST without stream/Accept unchanged (`TestChatSSE_JSONUnchangedWithoutStream`, existing `TestAgentsAndSessions` chat echo).
- Scripted provider two frames then DONE, concat `hello world` (`TestChatSSE_ScriptedTwoFramesThenDONE`).
- Missing session with stream flags still JSON 404 (`TestChatSSE_ErrorBeforeStreamJSON`).
- Quota 429 and injection 400 with stream flags stay JSON (`TestChatSSE_QuotaAndInjectionStayJSON`).
- Provider error: SSE `event: error`; JSON path 502 (`TestChatSSE_ProviderErrorEvent`).
- Direct `handleChat` / `handleChatWithLLM` wrappers and `/v1/chat` alias (`TestChatSSE_HandleChatAndLLMWrappers`, `TestChatSSE_V1Alias`).
- ChatPage resets `sending` when the selected session changes so the composer cannot stick after a mid-send switch.

## Non-goals

Streaming tokens from the upstream LLM into the gateway (039 already parses provider SSE when `OpenAI.Stream=true`), live bind/kill of demo ports, copying goclaw/ZaloCRM chat UIs.

# QA — SPEC 068 Real chat SSE (provider deltas)

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Closes **CTO-05** / matrix **R1**. Do not merge. Do not start SPEC 069.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Providers implement `Chat()` and `ChatStream()`; agent loop uses stream for token-by-token | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/02-providers.md` (Chat vs ChatStream, SSE sequence) |
| Stream retries only before the first content chunk | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/02-providers.md` |
| HTTP `POST /v1/chat/completions` `"stream": true` → SSE `data: {...}` terminated by `data: [DONE]` | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/18-http-api.md` §2 |

goso mapping (self-written): `POST /api/chat` (and `/v1/chat` alias) with `stream: true` or `Accept: text/event-stream` flushes `data: {"delta":...}` per provider callback, then `data: [DONE]`. JSON POST without those flags is unchanged. OpenAI-compat parses upstream `choices[0].delta.content` as bytes arrive. Anthropic parses `content_block_delta` text. Echo emits 1–N test chunks. Providers without a native stream emit **one** honest chunk after `Chat` (no mid-string splits).

## What changed

- `llm.StreamHandler` + `ChatStream` helper. Native streamers: Echo (1–N), OpenAI-compat (upstream SSE), Anthropic (`content_block_delta`). Others: one chunk after Chat.
- OpenAI `ChatStreamTools` streams text deltas and may return tool calls from the same turn. Scripted (ToolChat, no native stream) completes tools non-stream then emits the final text as one chunk.
- `POST /api/chat` stream path calls the provider/pipeline stream callback and flushes each delta. JSON path unchanged (`200` + `application/json`).
- Production `splitSSEDeltas` **deleted**. Client disconnect cancels the provider `context` (`r.Context()`).
- Control-plane `chatStream` already consumes SSE deltas; types unchanged.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 069. Do not break `GOSO_ENV=demo` router9 (JSON Chat still non-stream to upstream unless ChatStream is requested).

## Proof

- httptest OpenAI SSE with two delayed `data:` content chunks: gateway flushes the first delta **before** the second upstream chunk is written (`TestChatSSE_OpenAIFlushesFirstDeltaBeforeSecondUpstreamChunk`, `TestOpenAI_ChatStreamCallbacksBeforeSecondChunk`).
- Echo 3 chunks → 3 SSE `data` events + `[DONE]` (`TestChatSSE_EchoThreeChunksThenDONE`, `TestEcho_ChatStreamThreeParts`).
- Default Echo / Scripted stream: one honest chunk + `[DONE]` (`TestChatSSE_EchoAcceptOneHonestChunkThenDONE`, `TestChatSSE_ScriptedOneHonestChunkThenDONE`).
- Non-stream POST still JSON 200 (`TestChatSSE_JSONUnchangedWithoutStream`).
- Client disconnect cancels provider context (`TestChatSSE_ClientDisconnectCancelsProvider`).
- Anthropic `content_block_delta` (`TestAnthropic_ChatStreamContentBlockDelta`).
- `grep splitSSEDeltas` empty in non-test production files.

## Non-goals

SPEC 069 health chrome. Binding/killing demo ports. Merge. Copying goclaw Go. Fake mid-splits of completed replies.

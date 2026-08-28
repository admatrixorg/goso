# SPEC 068 — Real chat SSE (provider deltas)

> LOCKED after 067. Clean-room. No goclaw/ZaloCRM copy. Do not kill `:8082` `:8091`.

Closes **CTO-05** / matrix **R1** (and CP **D3** stream honesty). Audit: post-hoc `splitSSEDeltas` after full reply.

## GoClaw (cite, no copy)

- Providers implement `Chat()` and `ChatStream()`; loop uses stream for token-by-token (`docs/02-providers.md` interface + SSE sequence).
- HTTP `POST /v1/chat/completions` `"stream": true` → SSE `data: {...}` terminated by `data: [DONE]` (`docs/18-http-api.md` §2).
- Stream retries only before first content chunk (`docs/02-providers.md`).

## goso today

- `gateway/internal/httpapi/chat_sse.go` `writeChatSSE` splits the **completed** reply in half (`splitSSEDeltas`).
- CP `chatStream` already consumes SSE deltas (`control-plane/src/api/client.ts`).
- OpenAI adapter may already have a Stream flag internally (039) — default chat path does not forward provider chunks.

## goso plan (self-written)

1. Add `llm.StreamHandler func(delta string)` (or equivalent) on Provider. Echo emits 1–N chunks in tests. OpenAI-compat: parse upstream SSE `choices[0].delta.content` and callback **as bytes arrive**. Anthropic: `content_block_delta` text. Others: if no native stream, **one** chunk after Chat (honest, do not fake mid-splits).
2. Chat handler: if `stream`/Accept SSE, call Stream path and `sseWriter.data({"delta":...})` per callback, then `[DONE]`. JSON path unchanged.
3. **Delete** production use of `splitSSEDeltas`. Keep a test that proves two httptest flushes happen **before** the upstream handler finishes the full body (chunked OpenAI fixture).
4. Cancel: client disconnect stops the provider context.
5. Tools: if a provider cannot stream with tools, complete tools non-stream then stream the final text **or** document fail-closed — do not half-split.

## UI

Chat page already streams; verify deltas appear incrementally against Echo multi-chunk. No new page required. typecheck if types change.

## Tests

- httptest OpenAI SSE fixture with two `data:` content chunks delayed; gateway must flush first delta before second upstream chunk is written.
- Echo Stream 3 chunks → 3 SSE data events + DONE.
- Non-stream POST still JSON 200.
- `grep splitSSEDeltas` empty in non-test production files.

QC: typecheck, `go test ./...`, build, agpl 0, `docs/qa/068-real-sse.md`. Commit `admatrixmdp/spec068-real-sse`. Do not merge.

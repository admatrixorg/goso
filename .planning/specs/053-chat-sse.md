# SPEC 053 — Chat SSE in control-plane

> LOCKED: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

D3 PARTIAL: ChatPage POSTs `/api/chat` JSON non-stream. Gateway OpenAI client already parses SSE when `stream=true` (039).

## HTTP

- Existing `POST /api/chat` JSON **stays** (default).
- If request has `Accept: text/event-stream` **or** JSON body `"stream": true`: respond `text/event-stream` with events `data: {"delta":"..."}` then `data: [DONE]`. Same auth. Errors as JSON 4xx/5xx before stream, or one `event: error` line.
- Tests: httptest scripted/echo provider; client reads two frames then DONE. JSON POST without stream unchanged.

## UI

ChatPage: prefer fetch stream when available; append deltas to assistant bubble; keep StatusLine/error from 046. Fallback to JSON POST if stream 406/unsupported. i18n if needed. typecheck.

`docs/qa/053-chat-sse.md`. Commit `admatrixmdp/spec053-chat-sse`. Do not merge.

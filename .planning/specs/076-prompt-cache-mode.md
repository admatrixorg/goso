# SPEC 076 — Prompt cache_control + persisted prompt_mode

> LOCKED after SPEC 075 merge. Clean-room. Do not kill `:8082` `:8091`.

Closes matrix **Q2, Q3**. R1 leftover stream is already 068; do not re-split SSE.

## GoClaw (cite, no copy)

| Behavior | Cite |
|----------|------|
| Four prompt modes (full / task / minimal / none) with section gating | README Core Features; `docs/01-agent-loop.md` (system prompt mode) |
| Anthropic prompt caching / cache boundaries on stable prefix | goso 039 already documents CacheMode=full; GoClaw providers SSE+cache (`docs/02-providers.md` Anthropic native) |

## goso today

- 035: `POST /api/chat` `prompt_mode` request-only; unknown 400; not stored on session.
- 039: Anthropic `cache_control` **only** when `CacheMode=full` on system/last message.

## goso plan (self-written)

1. Persist `prompt_mode` on `sessions` (ALTER TEXT, default empty = full). `POST /api/chat` uses request `prompt_mode` if set; else session; else full. `PATCH /api/sessions/{id}` `{prompt_mode}`. Unknown → 400.
2. Cache: when Anthropic `CacheMode=full` (or new `GOSO_PROMPT_CACHE=full`), attach `cache_control: {type: ephemeral}` to **all stable prefix blocks** in the Messages payload (system + bootstrap files + last non-user cacheable block — document the exact list in QA). Other modes must **omit** cache_control (no crash). OpenAI-compat: if the body already has no cache field, leave it (no fake OpenAI cache).
3. CP: session editor or chat chrome select full/task/minimal/none; PATCH persist; i18n.
4. Tests: session round-trip; chat without field uses stored mode; Anthropic httptest sees cache_control in full and not in none.

## Non-goals

pgvector. Changing 068 SSE. Copying goclaw.

QC: typecheck, go test, build, agpl, agpl-docs.
`docs/qa/076-prompt-cache-mode.md`
Commit `admatrixmdp/spec076-prompt-cache-mode`. Do not merge. Do not start 077.

# QA — SPEC 140 Jev knowledge filter

No UI tab changes. No prototype screenshots.

## Insertion point

Bridge knowledge injection (`gateway/internal/bridge/knowledge_jev.go`). Agent routing is already a session, mention, or channel binding. Skills are loaded by BM25 `skill_search` / `use_skill`, not by stuffing the catalog into the prompt. The bridge pastes synced knowledge into every reply, up to 32 KiB, with no look at the customer message. That is the token sink this pilot filters.

## Checks

- `GOSO_JEV_ENABLED` unset or `GOSO_JEV_API_KEY` empty: `knowledgeBlock` equals `knowledgePrompt`.
- Context, questions, and thresholds are separate versioned JSON files. The cache key includes the model id and those versions. 429/529 back off inside the call budget. A Jev error keeps the unfiltered knowledge block.
- Fake Jev: a confident no drops that chunk; the passage is inside the question, not in `state`; a 502 keeps every chunk.
- Unit tests cover batching, the call cap, timeout, HTTP 500, cache TTL, SSRF when `GOSO_SSRF=1`, and the question file override.
- `scripts/jev-knowledge-eval.sh` skips with exit 0 when the key is unset. The 32 labels are existing Minh An (demo) strings only.

## Commands

```
go test ./gateway/...
cd control-plane && npm test && npm run verify && npm run qa:tabs
scripts/jev-knowledge-eval.sh
```

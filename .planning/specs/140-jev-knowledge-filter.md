# SPEC 140 — Jev relevance filter on bridge knowledge

Status: pilot. One insertion point. Do not merge from this spec.

## Insertion point

Score synced knowledge chunks in the Omni bridge (`knowledgeBlock`) before `knowledgePrompt` appends them to the system message.

Not agent routing. The session, an `@` mention, or the channel binding already names the agent. No prompt contains every agent's instructions, so a Jev call would add a hop and could send a customer to the wrong advisor.

Not skill selection. Skills are not in the prompt. `skill_search` is local BM25 (at most 5 hits) and `use_skill` loads one body only when the model asks. That path costs no tokens until the model chooses a skill.

Synced knowledge is the query-blind cost: every bridge reply pastes documents, in catalog order, up to `knowledge_max_bytes` (default 32 KiB) into the system message. Company Brain recall and the vault are already small or on demand (recall is capped; vault and the knowledge graph are tools). The bridge knowledge block is the wholesale injection.

## Behavior

- Off unless `GOSO_JEV_ENABLED` is on and `GOSO_JEV_API_KEY` is set. Otherwise the prompt bytes match today.
- One `noul` per chunk that already fits the cap. The chunk text is pasted into the question. `state` is only the versioned English context in `context.json`. Questions and thresholds are separate versioned files. A choice over chunks is not the live decision: choice probabilities sum to 1, and jev-1.13 documents a lean toward the first option. There is no jev-1.14 jaggedness page. The eval measures that lean with a reversed Choice; the filter stays on noul.
- Drop a chunk only when `noul` is at or below `no_at` (0.3 in the thresholds file, the docs' starting "no"). The band between `no_at` and `yes_at` (0.7) keeps the chunk for the chat model. Timeout, HTTP error after the 429/529 backoff, missing answer, a chunk over the per-question estimate, or a chunk past `GOSO_JEV_MAX_CALLS` keeps it. A panic or a short result also keeps today's prompt.
- Questions are batched. Default cap is 2 calls per reply. Cache key is model id + context version + question-set version + the file hashes + normalized message and passage, TTL `GOSO_JEV_CACHE_TTL` (default 24h). 429 and 529 retry with exponential backoff inside the call budget and honor Retry-After when it fits. The response `model` is logged.
- Model default is `jev-1.14.0`. `jev-latest` is an alias and is not the default.
- The key is read from the process environment only. It is not an overlay field, not a `VITE_` variable, and `GET /api/config` refuses a snapshot that contains it.

## Out of scope

Agent routing, skill choice, vault search, recall admission, and the reply-check Clef call. Thresholds are the documented starting band. They are not fitted.

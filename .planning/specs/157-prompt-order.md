# SPEC 157 — Prompt order, still one system message

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only, the same fixtures as SPEC 153. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 157. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

`Runner.attachPrompt` joins every instruction and every memory note into one system message. The comment in `gateway/internal/pipeline/runner.go` records why: some OpenAI-compatible routers drop every system message after the first, and that used to discard recall. The order today is agent prompt, then recall, then bootstrap, then context files, then the same-session summary, then history. Recall sits in the moving part of the prefix, so the prefix changes every time recall changes.

## Behavior

1. Keep a single system message in `attachPrompt` and in `ChannelSystem`. Do not emit a second system message.
2. Join the parts in this order. Empty parts are omitted, which is how the code already treats a blank recall note.
   1. System prompt, team note, and instructions.
   2. Bootstrap files.
   3. Context files, only when `GOSO_CONTEXT_FILES` is on (SPEC 150).
   4. Static profile, only when SPEC 162 is on. Empty until that spec lands.
   5. Dynamic profile, only when SPEC 162 is on. Empty until that spec lands.
   6. Cross-session recall, after SPEC 165 admission when that flag is on.
   7. The same-session `Previous summary:` line.
   8. History, still capped at 50.
3. `ChannelSystem` uses the same order and the same single message. It still does not add a same-session summary, because the channel already sends the thread.
4. The hashed prefix is parts 1–4 (through the static profile, or through context files while SPEC 162 is off). Two consecutive turns of the same Minh An session, with an unchanged static profile, have the same prefix hash. Recall and the dynamic profile are outside the hash.
5. `prompt_mode=none` still skips recall, bootstrap, context files, and profiles.

## Gate

- **Invariant.** There is only one system message.
- **Acceptance.** The hash of the prefix is stable across turns of the same session.
- **Negative test.** A router that drops a second system message still receives recall, because recall is inside the first message.
- **Rollback.** Revert the order. The single-message join stays either way.

## Non-goals

A second system message “for the cache”. A token cap. Caps are SPEC 165 only. Profile text is SPEC 162. This spec only reserves the slots.

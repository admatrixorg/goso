# SPEC 133 — Chat agent and model for one session

Status: implemented on `cursor/goso-spec-133-chat-pickers-a600`.
Base: `origin/main` at `012858a` (SPEC 132 not merged; this slice does not stack).
Gap: `docs/qa/goso-spec-2026-10-31-gap.md` §2.2.

## Why this slice

§13 phase 1 includes multi-model chat. Vault wiki, Docling, and Memory Facts are larger and Facts should stack on unmerged SPEC 132. The left-nav merge (SPEC 139) is real but it is not in the phase 1 measure, and it revises the locked SPEC 119 map, so it stays its own change. This slice is the phase 1 piece that is self-contained: the session already had an agent, the pipeline already resolved that agent's provider and model, and the composer did not expose either axis.

## Goal

In Chat, the operator picks the session agent and the session model. Model default is **Theo agent** (the effective agent's model). Typing `@` picks an agent for that message only; the session agent stays. Each assistant reply shows `tên agent · tên model`. Changing the session agent draws a thin line **Chuyển sang X**.

## Behavior

1. `PATCH /api/sessions/{id}` accepts any of `prompt_mode`, `agent_id`, `model_override`, `provider_override`. Prompt-only patches do not clear a model override. Empty model override means follow the agent. A provider without a model is 400. Unknown or other-tenant agents are 400. A paused agent is 409.
2. Changing `agent_id` appends a message `role=event` with the new agent id. Events are not sent to the model.
3. `POST /api/chat` accepts optional `agent_id`. When it differs from the session agent, that turn uses the mentioned agent's persona, tools, and (if no session model override) model. The session row is unchanged. The user message stores `agent_id` = session agent and `mention_agent_id` = the mention.
4. Assistant messages store `agent_id`, `model`, and `provider`. `model` is the provider's model after resolve (`ModelName`), so an Echo fallback is captioned `echo` rather than a model it did not run.
5. A session model override replaces the agent model and, when `provider_override` is set, the provider. The override still applies on an `@` turn.
6. Composer matches the 31/10 prototype: one rounded box, agent and model as quiet buttons, `@` popover, highlighted `@Tên`, muted caption, short switch line, centred greeting when the thread is empty. No prompt-mode control, stream badge, or “unavailable” footer on this screen. Prompt mode remains on the Sessions page. Ctrl+K opens the agent menu. File attach stays SPEC 134, so the composer has no dead upload button.

## Out of scope

Paste/drop (134). Greeting, stop, plain chat, retry, thumbs, save-to-vault, vision fallback (146, 145). Allowed-models lock on the agent. Nav merge (139). Personal-subscription providers.

## Acceptance

- Mention does not change `sessions.agent_id`.
- Theo agent sends the agent's model; a session override sends that model; the caption matches the model the provider ran.
- Switch line is an `event` row and is absent from the LLM payload.
- Unknown `@` agent is 400. Prompt-mode patch keeps `model_override`.
- SQLite reloads the override and the message stamps.
- `go test` for store, pipeline, agent, httpapi. Control-plane `npm test` and `npm run verify`.
- Vietnamese copy for the new strings. No OpenKnowledge code. No license change.

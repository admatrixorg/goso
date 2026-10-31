# QA — SPEC 133 chat agent and model pickers

Date: 2026-10-31. Clean-room Go and React. No OpenKnowledge import. No goclaw-source copy. No Claude CLI / Codex OAuth. No deploy or secret changes. SPEC 132 is not in this branch.

## What changed

- Session `model_override` / `provider_override`. Empty means Theo agent. `PATCH` can also move `agent_id` and then writes an `event` message.
- Chat `agent_id` is one turn. Session agent stays. User row stores the mention. Assistant row stores agent, model, and provider.
- `event` messages are dropped in `pipeline.ToLLM`.
- Chat composer: agent select, model select, `@` popover, accent `@Tên`, caption `agent · model`, switch line.

## Commands

```
gofmt -w (edited Go files)
go test ./gateway/... -count=1
cd control-plane && npm test && npm run verify
```

`npm test`: 317 pass, 0 fail. `npm run verify`: `tsc --noEmit` and vite build exit 0. `go test ./gateway/...` exit 0.

UI was exercised against a local gateway on 127.0.0.1:8080 with a local OpenAI-compatible mock (not a commercial provider). Screenshot: chat composer shows Agent and Model (Theo agent), an assistant caption, a "Chuyển sang" line, and an `@` mention. The mock reply is the text the local server returned.

## Not verified

Live Mac mini gateway, Docling, and a real provider key. Echo fallback captions the model as `echo` when no provider applies the override; the OpenAI-compat test covers the applied model.

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

UI was exercised in Chrome against a local gateway and a local OpenAI-compatible mock (reply text starts with `Mock local:`). The chat column was then restyled to the 31/10 prototype: greeting, 740px thread, one rounded composer, quiet agent/model buttons, hover-only copy, no stream badge and no “unavailable” footer. A second browser pass covered agent/model/@/switch/caption, empty state, long threads, streaming deltas, reload, session switch, 50 agents, a removed model name, no-agent and one-agent lists, light/dark, and widths 1280/1024/768/390. Offline shows “Không kết nối được máy chủ”. CRM `healthz`/`readyz` 500s on Overview are the existing proxy to `:8082`, which is not running here.

## Not verified

Live Mac mini gateway, Docling, and a real provider key. Echo fallback captions the model as `echo` when no provider applies the override; the OpenAI-compat test covers the applied model.

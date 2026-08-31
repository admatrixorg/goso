# QA — SPEC 131 inbound channels Resolve the bound agent's LLM

Date: 2026-08-31. Clean-room Go. No goclaw-source copy. No banned author ids. Worker does not merge and does not restart live demo. Demos `:3000` `:8082` `:8091` `:18791` were not bound or killed. No vendor token values in this record.

## Live surfaces inspected (worker, before repo edits)

Verified by the dispatch (operator Telegram, 2026-08-31):

| Surface | Observation |
| --- | --- |
| Telegram inbound reply | `LLM error: router9 429: ... [opencode-go/deepseek-v4-flash] ... GoUsageLimitError ... Monthly usage limit reached` |
| Agent `telegram` / Telegram Bot | `llm_provider=router9`, `model=gcli/grok-4.5`. Catalog binds Telegram → that agent. `dm_policy`: pairing. |
| Process fallback (`serve.go`) | env router9 model `ocg/deepseek-v4-flash` |
| Pairing / Control Plane | Not a pairing failure. Not a Control Plane 401. |

Cause: `telegram.go` `ingest` (and Zalo / `replyInbound`) used `t.LLM` / `d.LLM` / `z.LLM` without `llm.Resolve`. Webhook/chat already Resolve. Inbound did not, so Telegram hit DeepSeek quota.

Worker did not restart `:18080`. Advisor rebuilds `/tmp/goso-044-demo/goso-gateway` from merged main and restarts `:18080` only.

## What changed

`resolveInboundLLM` in `gateway/internal/channel/inbound.go` calls `llm.Resolve(st, agent.LLMProvider, agent.Model, fallback)` after the bound/ensure agent is known. Miss keeps fallback (Echo if nil). Wired into Telegram ingest, Zalo OA, Zalo Personal, and `replyInbound` (Discord/Slack/Feishu/WhatsApp). `llm.Resolve` contract unchanged. Pairing, tokens, CRM, Control Plane unchanged.

Tests: httptest OpenAI-compat records the `model` field. Agent `telegram` / `zalo-oa` with `LLMProvider=router9` and `model=gcli/grok-4.5` must send that model, not `ocg/deepseek-v4-flash`. Empty provider/model still Echo. Named miss (`nope`) keeps Echo and webhook 200. Bound `ChannelConfig.AgentID` wins over `ensureAgent()`.

## Checks

```
go test ./gateway/...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `go test ./gateway/internal/channel/`: pass (includes `TestTelegram_ResolvesAgentModel`, `TestTelegram_BoundAgentIDWins`, `TestTelegram_UnknownProviderKeepsFallback`, `TestZaloOA_ResolvesAgentModel`, `TestDiscord_ResolvesAgentModel`, helper empty/miss/apply). Existing Echo inbound tests stay green.
- `go test ./gateway/...`: pass (all gateway packages ok; `builtin` 21s, `httpapi` 10s, `channel` 4s).
- `agpl-check` and `agpl-check-docs`: exit 0.

## Out of scope

Merge `--no-ff` and gateway `:18080` rebuild/restart belong to advisor/CTO live QC. Vite `:3000`, CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched. Pairing, vendor tokens, Control Plane UI, default catalog model, and DeepSeek quota stay DI.

No credentials or secret values are included in this record.

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

## Advisor live QC

Date: 2026-08-31. After `worker_done` on `task_a375b8dee31d` / `ctx_b74f179fd98a`. Merged `--no-ff` as `766b051` (`Merge SPEC 131 inbound LLM resolve`) of `11ce616` + `8fd9990` onto `90e4d6d`. Clean-room Go. No goclaw-source copy. No banned author ids. No token literals in this record.

Advisor re-ran `go test ./gateway/internal/channel/` and `go test ./gateway/...` on the worker worktree before merge (all packages ok). `agpl-check` and `agpl-check-docs` exit 0. `resolveInboundLLM` is wired before `ChatUsage` on Telegram, Zalo OA, Zalo Personal, and `replyInbound`. httptest asserts agent model `gcli/grok-4.5`, not process fallback `ocg/deepseek-v4-flash`.

Rebuild: `go build -o /tmp/goso-044-demo/goso-gateway ./gateway/cmd/goso-gateway` from merged main. Restart **`:18080` only** (old pid `68421` → new pid `12422`). Same `GOSO_*` env names as the previous process (values not copied here). `GET /healthz` 200; `GET /api/agents` without bearer 401. Unchanged: Vite `:3000` pid `67690`, CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`. New binary contains `resolveInboundLLM`.

Live Telegram ping is an operator action (cannot inject a DM from advisor). Automated proof is the httptest model field. After restart, a new inbound should Resolve `gcli/grok-4.5` instead of quoting DeepSeek monthly quota.

### Advisor verdict: PASS — SPEC 131 closed (gateway rebuilt)

Do not spawn a second 131 worker. CRM `:8082`, sidecar `:8091`, and Vite `:3000` remain untouched.

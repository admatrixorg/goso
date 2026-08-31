# SPEC 131 — Channel inbound must Resolve the bound agent's LLM

Status: implemented (worker). Merge `--no-ff` and gateway `:18080` rebuild/restart stay with advisor/CTO after SPEC 130 advisor live QC PASS (`90e4d6d`).
Owner: Grok implementation; advisor/CTO merge and live QC.
Base: `origin/main` at `90e4d6d`.

## Goal

Telegram (and other inbound channels) must call the bound agent's LLM, not the process fallback. An operator who set `llm_provider=router9` and `model=gcli/grok-4.5` on agent `telegram` must not hit DeepSeek quota on inbound.

## Operator question

“Telegram replied `LLM error: router9 429: ... [opencode-go/deepseek-v4-flash] ... Monthly usage limit reached`. The agent is Grok. Why is inbound using DeepSeek?”

## Cause (verified)

Live agent `telegram` / display **Telegram Bot**:
- `llm_provider`: `router9`
- `model`: `gcli/grok-4.5`
- Channel catalog binds Telegram → that agent; `dm_policy`: pairing.

Process default provider (`serve.go`) is env router9 model **`ocg/deepseek-v4-flash`**.

`gateway/internal/channel/telegram.go` `ingest` did:

```
provider := t.LLM
reply, _, err := llm.ChatUsage(ctx, provider, msgs)
```

It never called `llm.Resolve(st, agent.LLMProvider, agent.Model, fallback)`. Same skip in `zalo_oa.go`, `zalo_personal.go`, and `inbound.go` `replyInbound` (Discord/Slack/Feishu/WhatsApp). Webhook/chat already Resolve (`handlers_webhooks.go`, `agent/runtime.go`). Inbound channels did not. Telegram therefore used the process fallback DeepSeek even though the operator set Grok on the agent.

This is an inbound Resolve skip, not pairing and not Control Plane 401.

## Behavior

1. After the bound/ensure agent is known, before `ChatUsage`:

```
provider := fallback // t.LLM / d.LLM / z.LLM; Echo if nil
if agent != nil {
  if p, err := llm.Resolve(st, agent.LLMProvider, agent.Model, provider); err == nil && p != nil {
    provider = p
  }
}
```

Implemented as `resolveInboundLLM` in `channel` and used by Telegram, Zalo OA, Zalo Personal, and `replyInbound`.

2. Resolve miss keeps fallback (webhook stays 200). LLM errors still become `LLM error: %v` reply text.
3. Bound `ChannelConfig.AgentID` still wins over `ensureAgent()` on Telegram; Resolve uses that agent.
4. Empty provider/model still uses the process fallback (Echo in tests).

## Constraints

- Do not change `llm.Resolve` contract.
- Do not change pairing, tokens, CRM, Control Plane.
- Do not invent a second router9 provider. Do not copy goclaw.
- Worker does not merge and does not restart live demo. Never kill `:3000` `:8082` `:8091` `:18791`. Advisor rebuilds `/tmp/goso-044-demo/goso-gateway` from merged main and restarts **`:18080` only** with the same env.

## Non-goals

- Buying more DeepSeek quota.
- Changing the default catalog model.
- Control Plane UI.
- Channel vendor tokens.
- Merge `--no-ff` and live `:18080` restart (advisor/CTO).

## Acceptance criteria

1. Telegram ingest Resolves the bound/ensure agent's `LLMProvider` + `Model` before `ChatUsage`.
2. Zalo OA / Zalo Personal / Discord-Slack-Feishu-WhatsApp (`replyInbound`) do the same.
3. Resolve miss keeps fallback; webhook is 200; LLM errors remain reply text.
4. Tests: agent `telegram` with `LLMProvider=router9` (or openai-compat) + non-default model; httptest OpenAI-compat sees that model, not the process fallback. Empty provider/model still Echo. Bound `ChannelConfig.AgentID` wins over `ensureAgent()`. At least one of inbound/zalo covered.
5. `go test ./gateway/...` pass.
6. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0.
7. Delivery is two commits (fix vs docs). Merge stays with advisor/CTO.

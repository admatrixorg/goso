# QA — SPEC 042 Observability spans + optional OTLP

Date: 2026-08-27. Clean-room. Closes **O1–O3**. Grafana SaaS keys = **DI-18** (not in git). Jaeger collector URL = **DI-10** — optional env, no default cloud vendor. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No goclaw copy. No product secrets in git.

## What changed

Nested in-memory spans on a chat run: `agent` parent, children `llm` and `tool` (siblings under agent, not tool-under-llm). Persisted in the observe span-tree ring and on `ChatResult.spans`. `GET /api/traces` returns `{traces, spans}` — existing LLM traces stay; `spans` is an array of `{trace_id, spans:[…]}`.

Prompt-cache counter: `cache_read_tokens` defaults to **0** on `llm.Usage`, LLM traces, and every span. Anthropic Messages JSON `usage.cache_read_input_tokens` (or `cache_read_tokens`) is parsed when present.

OTLP: `GOSO_OTEL_ENDPOINT` empty → `NoopExporter` (tests, default). If set, thin HTTP JSON exporter POSTs an OTLP-shaped payload. Tests use `FakeExporter`. No `go.opentelemetry.io` dependency. No Grafana Cloud keys or vendor Authorization headers.

JSON access log middleware and `GET /metrics` / `GET /api/stats` are unchanged (SPEC 008).

## Commands

```
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Nested agent/llm/tool (`TestStartSpan_AgentParentLLMAndToolChildren`, `TestChat_NestedSpans`, `TestChat_EchoSpansCacheReadDefaultZero`).
- Wrap does not invent ToolChat on Anthropic/Echo (`TestWrap_PreservesToolChatOnlyWhenInnerHasIt`); wrapped Anthropic `cache_read_tokens=4` on the llm span (`TestChat_WrappedAnthropicCacheReadOnLLMSpan`).
- Nested spawn/delegate-style ctx does not reuse the parent collector (`TestStartSpan_IsolatesNewCollectorPerRun`, `TestChat_NestedRunIsolatesCollector`).
- Failed chat still persists a span tree (`TestChat_ErrorStillRecordsSpans`).
- `GET /api/traces` includes `spans` (`TestHandleTraces`, `TestHandleTraces_SpanTrees`).
- `cache_read_tokens` default 0; Anthropic parse (`TestAnthropic_UsageFromProvider`, `TestAnthropic_CacheReadTokens`, `TestEstimateUsage_WhenProviderOmits`).
- Empty `GOSO_OTEL_ENDPOINT` is noop; fake records; HTTP JSON has no Grafana header (`TestExporterFromEnv_EmptyIsNoop`, `TestFakeExporter_Records`, `TestHTTPExporter_PostsJSONNoGrafanaHeader`, `TestNew_UsesNoopWhenEndpointEmpty`). HTTP export is async so `/api/chat` is not blocked.
- JSON log + `/metrics` still green (`TestLogMiddleware_GeneratesAndEchoesRequestID`, `TestHandleStatsAndMetrics`).

## Non-goals

Grafana Cloud (DI-18), default Jaeger/Grafana vendor URL, protobuf OTLP SDK, Prometheus full exporter, alerting dashboards.

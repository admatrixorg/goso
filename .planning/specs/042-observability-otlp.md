# SPEC 042 — Observability spans + optional OTLP

> LOCKED: 2026-08-27. Closes **O1–O3**. Grafana SaaS = DI-18. Jaeger collector URL = DI-10 — **optional env**, no default cloud vendor.

## Goal

Nested in-memory **spans** on a chat run: `agent` parent, children `llm`, `tool`. Persist in existing observe ring **or** a small span slice on `ChatResult` / `GET /api/traces`. Prompt-cache counters: `cache_read_tokens` field default 0 (Anthropic parse if JSON has it).

OTLP: if `GOSO_OTEL_ENDPOINT` empty, no export (tests). If set, export spans via otel HTTP/protobuf **or** a thin exporter interface with a recording fake in tests (preferred: interface + noop + fake, so we do not add heavy deps unless already in go.mod). Do **not** add Grafana Cloud keys.

Keep JSON access log + `/metrics` (already CÓ).

## QC

`go test ./...`, build, agpl 0, `docs/qa/042-observability.md`. Commit, do not merge.

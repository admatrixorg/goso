import { jsonFetch } from "./client";

export type LlmTrace = {
  ts?: string;
  provider?: string;
  model?: string;
  latency_ms?: number;
  error?: string;
  request_id?: string;
  tokens?: number;
  cache_read_tokens?: number;
};

export type TraceSpan = {
  trace_id?: string;
  span_id?: string;
  parent_id?: string;
  kind?: string;
  name?: string;
  start?: string;
  end?: string;
  latency_ms?: number;
  status?: string;
  error?: string;
  cache_read_tokens?: number;
  attributes?: Record<string, string>;
};

export type SpanTree = { trace_id?: string; spans?: TraceSpan[] };

function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function num(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}

function traceOf(row: unknown): LlmTrace {
  const r = asObj(row);
  return {
    ts: str(r.ts) || undefined,
    provider: str(r.provider) || undefined,
    model: str(r.model) || undefined,
    latency_ms: num(r.latency_ms),
    error: str(r.error) || undefined,
    request_id: str(r.request_id) || undefined,
    tokens: num(r.tokens),
    cache_read_tokens: num(r.cache_read_tokens),
  };
}

function spanOf(row: unknown): TraceSpan {
  const r = asObj(row);
  const attrs = asObj(r.attributes);
  const attributes: Record<string, string> = {};
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "string") attributes[k] = v;
  }
  return {
    trace_id: str(r.trace_id) || undefined,
    span_id: str(r.span_id) || undefined,
    parent_id: str(r.parent_id) || undefined,
    kind: str(r.kind) || undefined,
    name: str(r.name) || undefined,
    start: str(r.start) || undefined,
    end: str(r.end) || undefined,
    latency_ms: num(r.latency_ms),
    status: str(r.status) || undefined,
    error: str(r.error) || undefined,
    cache_read_tokens: num(r.cache_read_tokens),
    attributes: Object.keys(attributes).length ? attributes : undefined,
  };
}

function treeOf(row: unknown): SpanTree {
  const r = asObj(row);
  const raw = Array.isArray(r.spans) ? r.spans : [];
  return { trace_id: str(r.trace_id) || undefined, spans: raw.map(spanOf) };
}

export const tracesApi = {
  list: async (limit?: number) => {
    const qs = limit ? `?limit=${encodeURIComponent(String(limit))}` : "";
    const j = asObj(await jsonFetch<unknown>(`/api/traces${qs}`));
    const traces = Array.isArray(j.traces) ? j.traces.map(traceOf) : [];
    const spans = Array.isArray(j.spans) ? j.spans.map(treeOf) : [];
    return { traces, spans };
  },
};

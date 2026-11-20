import { jsonFetch } from "./client";
import { capText, ERROR_CAP, publicAttrs, type ErrorGroup, type TraceItem, type TraceSpan } from "./traces-ops";

export type { ErrorGroup, TraceItem, TraceSpan };

export type LlmTrace = {
  ts?: string;
  provider?: string;
  model?: string;
  latency_ms?: number;
  error?: string;
  request_id?: string;
  trace_id?: string;
  agent_id?: string;
  status?: string;
  tokens?: number;
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
};

export type SpanTree = { trace_id?: string; spans?: TraceSpan[] };

export type TraceListQuery = {
  q?: string;
  agent?: string;
  channel?: string;
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export type TraceList = {
  items: TraceItem[];
  traces: LlmTrace[];
  spans: SpanTree[];
  total: number;
  offset: number;
  limit: number;
  truncated: boolean;
  error_groups: ErrorGroup[];
};

export type TraceDetail = {
  trace_id: string;
  item?: TraceItem;
  spans: TraceSpan[];
  truncated: boolean;
};

function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function num(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}

function bool(v: unknown): boolean {
  return v === true;
}

function itemOf(row: unknown): TraceItem | null {
  const r = asObj(row);
  const id = str(r.trace_id);
  if (!id) return null;
  return {
    trace_id: id,
    ts: str(r.ts) || undefined,
    status: str(r.status) || undefined,
    agent_id: str(r.agent_id) || undefined,
    channel: str(r.channel) || undefined,
    session_id: str(r.session_id) || undefined,
    provider: str(r.provider) || undefined,
    model: str(r.model) || undefined,
    latency_ms: num(r.latency_ms),
    input_tokens: num(r.input_tokens),
    output_tokens: num(r.output_tokens),
    cache_read_tokens: num(r.cache_read_tokens),
    error: capText(str(r.error), ERROR_CAP) || undefined,
    error_count: num(r.error_count),
  };
}

function traceOf(row: unknown): LlmTrace {
  const r = asObj(row);
  return {
    ts: str(r.ts) || undefined,
    provider: str(r.provider) || undefined,
    model: str(r.model) || undefined,
    latency_ms: num(r.latency_ms),
    error: capText(str(r.error), ERROR_CAP) || undefined,
    request_id: str(r.request_id) || undefined,
    trace_id: str(r.trace_id) || undefined,
    agent_id: str(r.agent_id) || undefined,
    status: str(r.status) || undefined,
    tokens: num(r.tokens),
    input_tokens: num(r.input_tokens),
    output_tokens: num(r.output_tokens),
    cache_read_tokens: num(r.cache_read_tokens),
  };
}

function spanOf(row: unknown): TraceSpan {
  const r = asObj(row);
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
    error: capText(str(r.error), ERROR_CAP) || undefined,
    input_tokens: num(r.input_tokens),
    output_tokens: num(r.output_tokens),
    cache_read_tokens: num(r.cache_read_tokens),
    attributes: publicAttrs(asStringMap(r.attributes)),
  };
}

function asStringMap(v: unknown): Record<string, string> | undefined {
  const r = asObj(v);
  const out: Record<string, string> = {};
  for (const [k, val] of Object.entries(r)) {
    if (typeof val === "string") out[k] = val;
  }
  return Object.keys(out).length ? out : undefined;
}

function treeOf(row: unknown): SpanTree {
  const r = asObj(row);
  const raw = Array.isArray(r.spans) ? r.spans : [];
  return { trace_id: str(r.trace_id) || undefined, spans: raw.map(spanOf) };
}

function itemFromTree(tree: SpanTree): TraceItem | null {
  const id = tree.trace_id || "";
  if (!id) return null;
  const spans = tree.spans || [];
  const agent = spans.find((s) => s.kind === "agent");
  const llm = spans.find((s) => s.kind === "llm");
  const err = spans.find((s) => s.error || s.status === "error");
  return {
    trace_id: id,
    ts: agent?.start || spans[0]?.start,
    status: err ? "error" : "ok",
    agent_id: agent?.attributes?.agent_id,
    channel: agent?.attributes?.channel,
    session_id: agent?.attributes?.session_id,
    provider: llm?.name,
    model: llm?.attributes?.model,
    latency_ms: agent?.latency_ms ?? spans.reduce((m, s) => Math.max(m, s.latency_ms || 0), 0),
    input_tokens: spans.reduce((n, s) => n + (s.input_tokens || 0), 0),
    output_tokens: spans.reduce((n, s) => n + (s.output_tokens || 0), 0),
    cache_read_tokens: spans.reduce((n, s) => n + (s.cache_read_tokens || 0), 0),
    error: err?.error,
  };
}

function itemFromLlm(tr: LlmTrace, i: number): TraceItem | null {
  const id = tr.trace_id || tr.request_id || "";
  if (!id) return null;
  return {
    trace_id: id || `llm-${i}`,
    ts: tr.ts,
    status: tr.status || (tr.error ? "error" : "ok"),
    agent_id: tr.agent_id,
    provider: tr.provider,
    model: tr.model,
    latency_ms: tr.latency_ms,
    input_tokens: tr.input_tokens ?? tr.tokens,
    output_tokens: tr.output_tokens,
    cache_read_tokens: tr.cache_read_tokens,
    error: tr.error,
  };
}

function groupOf(row: unknown): ErrorGroup | null {
  const r = asObj(row);
  const message = str(r.message);
  const count = num(r.count);
  if (!message || count == null) return null;
  return { message, count };
}

function deriveItems(traces: LlmTrace[], spans: SpanTree[]): TraceItem[] {
  const fromTrees = spans.map(itemFromTree).filter((x): x is TraceItem => x != null);
  if (fromTrees.length) return fromTrees;
  return traces.map(itemFromLlm).filter((x): x is TraceItem => x != null);
}

export const tracesApi = {
  list: async (q: TraceListQuery = {}): Promise<TraceList> => {
    const qs = new URLSearchParams();
    if (q.q) qs.set("q", q.q);
    if (q.agent) qs.set("agent", q.agent);
    if (q.channel) qs.set("channel", q.channel);
    if (q.status) qs.set("status", q.status);
    if (q.from) qs.set("from", q.from);
    if (q.to) qs.set("to", q.to);
    if (q.limit) qs.set("limit", String(q.limit));
    if (q.offset) qs.set("offset", String(q.offset));
    const path = qs.toString() ? `/api/traces?${qs}` : "/api/traces";
    const j = asObj(await jsonFetch<unknown>(path));
    const traces = Array.isArray(j.traces) ? j.traces.map(traceOf) : [];
    const spans = Array.isArray(j.spans) ? j.spans.map(treeOf) : [];
    const parsed = Array.isArray(j.items) ? j.items.map(itemOf).filter((x): x is TraceItem => x != null) : [];
    const items = parsed.length ? parsed : deriveItems(traces, spans);
    return {
      items,
      traces,
      spans,
      total: num(j.total) ?? items.length,
      offset: num(j.offset) ?? 0,
      limit: num(j.limit) ?? items.length,
      truncated: bool(j.truncated),
      error_groups: Array.isArray(j.error_groups)
        ? j.error_groups.map(groupOf).filter((x): x is ErrorGroup => x != null)
        : [],
    };
  },
  get: async (id: string): Promise<TraceDetail> => {
    const j = asObj(await jsonFetch<unknown>(`/api/traces/${encodeURIComponent(id)}`));
    const spans = Array.isArray(j.spans) ? j.spans.map(spanOf) : [];
    return {
      trace_id: str(j.trace_id) || id,
      item: itemOf(j.item) || undefined,
      spans,
      truncated: bool(j.truncated),
    };
  },
};

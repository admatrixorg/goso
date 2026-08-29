export const PAGE_SIZE = 20;
export const ATTR_CAP = 120;
export const ERROR_CAP = 400;

export type TimeRange = "15m" | "1h" | "24h" | "7d" | "all";

export type TraceItem = {
  trace_id: string;
  ts?: string;
  status?: string;
  agent_id?: string;
  channel?: string;
  session_id?: string;
  provider?: string;
  model?: string;
  latency_ms?: number;
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  error?: string;
  error_count?: number;
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
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  attributes?: Record<string, string>;
};

export type ErrorGroup = { message: string; count: number };

const SECRET_KEYS = [
  "prompt",
  "messages",
  "content",
  "arguments",
  "tool_input",
  "tool_result",
  "result",
  "token",
  "password",
  "secret",
  "authorization",
  "api_key",
  "apikey",
  "hmac",
  "private_key",
  "bot_token",
  "access_token",
];

export function parseTraceHash(hash: string): string {
  const m = /^#traces(?:\/([A-Za-z0-9_-]+))?$/.exec((hash || "").trim());
  return m?.[1] || "";
}

export function tracesHash(id?: string): string {
  const v = (id || "").trim();
  return v ? `#traces/${v}` : "#traces";
}

export function rangeFrom(range: TimeRange, now = Date.now()): string | undefined {
  const ms: Record<Exclude<TimeRange, "all">, number> = {
    "15m": 15 * 60_000,
    "1h": 3_600_000,
    "24h": 86_400_000,
    "7d": 7 * 86_400_000,
  };
  if (range === "all") return undefined;
  return new Date(now - ms[range]).toISOString();
}

export function statusOf(row: { status?: string; error?: string }): "ok" | "error" {
  if ((row.status || "").toLowerCase() === "error" || (row.error || "").trim()) return "error";
  return "ok";
}

export function tokenTotal(row: TraceItem): number {
  return (row.input_tokens || 0) + (row.output_tokens || 0);
}

export function uniqueValues(items: TraceItem[], key: "agent_id" | "channel"): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const it of items) {
    const v = (it[key] || "").trim();
    if (!v || seen.has(v)) continue;
    seen.add(v);
    out.push(v);
  }
  return out;
}

export function groupErrors(items: TraceItem[]): ErrorGroup[] {
  const counts = new Map<string, number>();
  for (const it of items) {
    const msg = (it.error || "").trim();
    if (!msg) continue;
    counts.set(msg, (counts.get(msg) || 0) + 1);
  }
  return [...counts.entries()]
    .map(([message, count]) => ({ message, count }))
    .sort((a, b) => b.count - a.count || a.message.localeCompare(b.message))
    .slice(0, 8);
}

export function childrenOf(spans: TraceSpan[], parentId: string): TraceSpan[] {
  return spans.filter((s) => (s.parent_id || "") === parentId);
}

export function spanRoots(spans: TraceSpan[]): TraceSpan[] {
  const roots = spans.filter((s) => !s.parent_id);
  return roots.length ? roots : spans.slice(0, 1);
}

export function capText(s: string, n: number): string {
  const v = (s || "").replace(/\u0000/g, "");
  const runes = Array.from(v);
  if (runes.length <= n) return v;
  return `${runes.slice(0, n).join("")}…`;
}

export function publicAttrs(attrs?: Record<string, string>): Record<string, string> | undefined {
  if (!attrs) return undefined;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(attrs)) {
    const lk = k.toLowerCase();
    if (SECRET_KEYS.some((sk) => lk === sk || lk.endsWith(`_${sk}`) || lk.startsWith(`${sk}_`))) continue;
    out[k] = capText(String(v ?? ""), ATTR_CAP);
  }
  return Object.keys(out).length ? out : undefined;
}

export function publicHasSecrets(row: unknown): boolean {
  const s = JSON.stringify(row ?? "");
  if (/"prompt"\s*:/.test(s)) return true;
  if (/"tool_input"\s*:/.test(s) || /"tool_result"\s*:/.test(s) || /"arguments"\s*:/.test(s)) return true;
  if (/\bsk-[A-Za-z0-9_-]{8,}\b/.test(s)) return true;
  if (/Bearer\s+[A-Za-z0-9._\-+=/]{8,}/i.test(s)) return true;
  return false;
}

export function pageLabel(offset: number, limit: number, total: number): { from: number; to: number; pages: number } {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, limit)));
  if (total <= 0) return { from: 0, to: 0, pages };
  const from = offset + 1;
  const to = Math.min(offset + limit, total);
  return { from, to, pages };
}

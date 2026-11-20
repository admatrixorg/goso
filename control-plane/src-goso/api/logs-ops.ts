import {
  classifyPageState,
  inventoryBlocksMutation,
  isFilteredEmpty,
  type PageLoadKind,
  type PageState,
} from "./page-state.ts";
import type { StreamConn } from "./events-ops.ts";

export const LIVE_CAP = 200;
export const MESSAGE_CAP = 400;

export const LOG_LEVELS = ["debug", "info", "warn", "error"] as const;
export type LogLevel = (typeof LOG_LEVELS)[number];

export const LOG_COMPONENTS = ["http", "llm", "gateway", "otel", "channel", "connector", "agent", "cron", "auth"] as const;

export type GatewayLog = {
  seq?: number;
  ts: string;
  level: string;
  component: string;
  message: string;
  request_id?: string;
};

export type LogFilters = {
  component?: string;
  q?: string;
  levels?: readonly string[];
};

export type { StreamConn };

const SECRET_KEYS = [
  "token",
  "password",
  "secret",
  "authorization",
  "api_key",
  "apikey",
  "bearer",
  "credential",
  "hmac",
  "private_key",
  "bot_token",
  "access_token",
];

const TOKEN_SHAPE = /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|wh_[A-Za-z0-9]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,})/i;
const ASSIGNMENT_SHAPE = /\b(token|password|secret|authorization|api[_-]?key|bearer|credential|hmac|private[_-]?key|bot[_-]?token|access[_-]?token)\s*[:=]\s*\S+/i;

export function capText(s: string, n: number): string {
  const v = (s || "").replace(/\u0000/g, "");
  if (v.length <= n) return v;
  return `${v.slice(0, n)}…`;
}

function hideKey(k: string): boolean {
  const lk = k.toLowerCase();
  return SECRET_KEYS.some((sk) => lk === sk || lk.includes(sk));
}

function dropSecrets(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(dropSecrets);
  if (v && typeof v === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, child] of Object.entries(v as Record<string, unknown>)) {
      if (hideKey(k)) continue;
      out[k] = dropSecrets(child);
    }
    return out;
  }
  if (typeof v === "string") return TOKEN_SHAPE.test(v) ? "[redacted]" : v;
  return v;
}

export function publicMessage(raw: string): string {
  let s = capText(String(raw || ""), MESSAGE_CAP * 2);
  s = s.replace(TOKEN_SHAPE, "[redacted]");
  s = s.replace(ASSIGNMENT_SHAPE, "$1=[redacted]");
  try {
    const parsed = JSON.parse(s) as unknown;
    s = JSON.stringify(dropSecrets(parsed));
  } catch {
    /* keep text */
  }
  return capText(s, MESSAGE_CAP);
}

export function publicHasSecrets(row: unknown): boolean {
  const parts: string[] = [JSON.stringify(row ?? "")];
  if (row && typeof row === "object") {
    const rec = row as Record<string, unknown>;
    if (typeof rec.message === "string") parts.push(rec.message);
  }
  for (const s of parts) {
    if (TOKEN_SHAPE.test(s)) return true;
    if (ASSIGNMENT_SHAPE.test(s) && !/=\s*\[redacted\]/i.test(s)) return true;
    if (/"token"\s*:\s*"[^"]+"/i.test(s) && !/"token"\s*:\s*"\[redacted\]"/i.test(s)) return true;
  }
  return false;
}

export function asPublicLog(raw: GatewayLog | null | undefined): GatewayLog | null {
  if (!raw || typeof raw !== "object") return null;
  const message = publicMessage(raw.message || "");
  const row: GatewayLog = {
    seq: typeof raw.seq === "number" ? raw.seq : undefined,
    ts: String(raw.ts || ""),
    level: normalizeLevel(raw.level),
    component: String(raw.component || "gateway"),
    message,
    request_id: raw.request_id ? String(raw.request_id) : undefined,
  };
  if (publicHasSecrets(row)) return null;
  return row;
}

export function normalizeLevel(s: string | undefined): LogLevel {
  switch (String(s || "").toLowerCase()) {
    case "debug":
    case "dbg":
    case "trace":
      return "debug";
    case "warn":
    case "warning":
      return "warn";
    case "error":
    case "err":
    case "fatal":
    case "panic":
      return "error";
    default:
      return "info";
  }
}

export function logKey(row: GatewayLog, fallback = 0): string {
  if (typeof row.seq === "number" && row.seq > 0) return `seq:${row.seq}`;
  return `${row.ts || ""}:${row.component || ""}:${fallback}`;
}

export function mergeLive(existing: GatewayLog[], incoming: GatewayLog, cap = LIVE_CAP): GatewayLog[] {
  const next = asPublicLog(incoming);
  if (!next) return existing;
  const key = logKey(next);
  const out = [next, ...existing.filter((e) => logKey(e) !== key)];
  return out.slice(0, cap);
}

export function applyFilters(rows: GatewayLog[], f: LogFilters): GatewayLog[] {
  const levels = f.levels?.map((x) => normalizeLevel(x));
  const needle = (f.q || "").trim().toLowerCase();
  return rows.filter((e) => {
    if (f.component && e.component !== f.component) return false;
    if (levels && levels.length > 0 && levels.length < LOG_LEVELS.length && !levels.includes(normalizeLevel(e.level))) return false;
    if (needle) {
      const hay = `${e.message} ${e.component} ${e.request_id || ""} ${e.level}`.toLowerCase();
      if (!hay.includes(needle)) return false;
    }
    return true;
  });
}

export function uniqueComponents(rows: GatewayLog[], extra: string[] = []): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const c of [...extra, ...rows.map((r) => r.component)]) {
    const s = (c || "").trim();
    if (!s || seen.has(s)) continue;
    seen.add(s);
    out.push(s);
  }
  return out;
}

export function toggleLevel(current: LogLevel[], level: LogLevel): LogLevel[] {
  if (current.includes(level)) return current.filter((x) => x !== level);
  return [...LOG_LEVELS].filter((x) => x === level || current.includes(x));
}

export function logsFiltersActive(f: LogFilters): boolean {
  const levels = f.levels ?? [];
  const levelNarrow = levels.length > 0 && levels.length < LOG_LEVELS.length;
  return Boolean((f.component || "").trim() || (f.q || "").trim() || levelNarrow);
}

export function classifyLogsHistory(input: {
  loading: boolean;
  loaded: boolean;
  error: unknown | null | undefined;
  itemCount: number;
}): PageState {
  return classifyPageState({
    loading: input.loading,
    loaded: input.loaded,
    error: input.error,
    itemCount: input.itemCount,
    keepStale: input.loaded && input.itemCount > 0,
  });
}

export function logsFilteredEmpty(state: PageState, unfilteredCount: number, visibleCount: number, filtersOn: boolean): boolean {
  if (isFilteredEmpty(state, unfilteredCount, visibleCount)) return true;
  return state.kind === "empty" && filtersOn;
}

export function logsActionsBlocked(kind: PageLoadKind): boolean {
  return inventoryBlocksMutation(kind);
}

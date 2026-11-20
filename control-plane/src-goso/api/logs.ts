import { jsonFetch, TENANT_STORAGE_KEY } from "./client";
import { gatewayFetchInit } from "./gateway-http";
import { parseSseBlock } from "./events-ops";
import { asPublicLog, type GatewayLog } from "./logs-ops";

export type { GatewayLog } from "./logs-ops";

export type LogsQuery = {
  component?: string;
  q?: string;
  level?: string;
  limit?: number;
  after?: number;
};

export type LogsList = { logs: GatewayLog[]; components: string[] };

function qs(q?: LogsQuery): string {
  const p = new URLSearchParams();
  if (q?.component) p.set("component", q.component);
  if (q?.q) p.set("q", q.q);
  if (q?.level) p.set("level", q.level);
  if (q?.limit) p.set("limit", String(q.limit));
  if (q?.after) p.set("after", String(q.after));
  const s = p.toString();
  return s ? `?${s}` : "";
}

function gatewayBase(): string {
  const raw = (import.meta.env.VITE_GATEWAY_URL as string) || "";
  return raw.replace(/\/$/, "");
}

function authHeaders(): Record<string, string> {
  const t = (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || localStorage.getItem("goso_token") || "";
  const h: Record<string, string> = { Accept: "text/event-stream" };
  if (t) h.Authorization = `Bearer ${t}`;
  try {
    const tenant = (localStorage.getItem(TENANT_STORAGE_KEY) || "").trim();
    if (tenant) h["X-Goso-Tenant"] = tenant;
  } catch {
    /* private mode */
  }
  return h;
}

export const logsApi = {
  list: async (q?: LogsQuery): Promise<LogsList> => {
    const j = await jsonFetch<LogsList>(`/api/logs${qs(q)}`);
    const logs = (j.logs ?? []).map(asPublicLog).filter((e): e is GatewayLog => Boolean(e));
    return { logs, components: Array.isArray(j.components) ? j.components.map(String) : [] };
  },

  stream: async (
    onEvent: (e: GatewayLog) => void,
    onReady: () => void,
    signal: AbortSignal,
    after?: number,
  ): Promise<void> => {
    const headers = { ...authHeaders() };
    if (after && after > 0) headers["Last-Event-ID"] = String(after);
    const res = await fetch(`${gatewayBase()}/api/logs/stream${qs({ after })}`, gatewayFetchInit({ headers, signal }));
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`${res.status} ${text}`);
    }
    if (!res.body) throw new Error("stream unsupported");
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = "";
    const flush = (raw: string) => {
      const { event, data } = parseSseBlock(raw);
      if (!data && !event) return;
      if (event === "ready") {
        onReady();
        return;
      }
      if (event === "error") {
        let msg = data;
        try {
          const j = JSON.parse(data) as { error?: string };
          if (j.error) msg = j.error;
        } catch {
          /* keep */
        }
        throw new Error(msg);
      }
      if (event && event !== "log") return;
      try {
        const parsed = asPublicLog(JSON.parse(data) as GatewayLog);
        if (parsed) onEvent(parsed);
      } catch {
        /* ignore keep-alives */
      }
    };
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true }).replace(/\r\n/g, "\n").replace(/\r/g, "\n");
        let idx: number;
        while ((idx = buf.indexOf("\n\n")) >= 0) {
          const raw = buf.slice(0, idx);
          buf = buf.slice(idx + 2);
          flush(raw);
        }
      }
    } finally {
      reader.releaseLock();
    }
  },
};

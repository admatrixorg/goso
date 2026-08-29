import { jsonFetch, TENANT_STORAGE_KEY } from "./client";
import { asPublicEvent, parseSseBlock, type GatewayEvent } from "./events-ops";

export type { GatewayEvent } from "./events-ops";

export type EventsQuery = {
  kind?: string;
  connector?: string;
  type?: string;
  actor?: string;
  limit?: number;
  after?: number;
};

export type EventsList = { events: GatewayEvent[] };

function qs(q?: EventsQuery): string {
  const p = new URLSearchParams();
  if (q?.kind) p.set("kind", q.kind);
  if (q?.connector) p.set("connector", q.connector);
  if (q?.type) p.set("type", q.type);
  if (q?.actor) p.set("actor", q.actor);
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

export const eventsApi = {
  list: async (q?: EventsQuery): Promise<EventsList> => {
    const j = await jsonFetch<EventsList>(`/api/events${qs(q)}`);
    const events = (j.events ?? []).map(asPublicEvent).filter((e): e is GatewayEvent => Boolean(e));
    return { events };
  },

  stream: async (
    q: EventsQuery | undefined,
    onEvent: (e: GatewayEvent) => void,
    onReady: () => void,
    signal: AbortSignal,
    after?: number,
  ): Promise<void> => {
    const headers = { ...authHeaders() };
    if (after && after > 0) headers["Last-Event-ID"] = String(after);
    const res = await fetch(`${gatewayBase()}/api/events/stream${qs({ ...q, after })}`, { headers, signal });
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
      if (event && event !== "ops") return;
      try {
        const parsed = asPublicEvent(JSON.parse(data) as GatewayEvent);
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

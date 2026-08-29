export type GatewayStatsProbe = {
  status: number;
  uptimeSeconds: number;
  requestCount: number;
  llmCallCount: number;
  wsUp: boolean;
  lastHeartbeat: string;
};

export function emptyGatewayStats(status = 0): GatewayStatsProbe {
  return { status, uptimeSeconds: 0, requestCount: 0, llmCallCount: 0, wsUp: false, lastHeartbeat: "" };
}

function asCount(v: unknown): number {
  if (typeof v === "number" && Number.isFinite(v)) return Math.max(0, Math.trunc(v));
  if (typeof v === "string" && v.trim()) {
    const n = Number(v);
    if (Number.isFinite(n)) return Math.max(0, Math.trunc(n));
  }
  return 0;
}

/** Parse GET /api/stats JSON. Unknown keys (including any secret-shaped fields) are ignored. */
export function parseStatsBody(body: unknown, status: number): GatewayStatsProbe {
  const out = emptyGatewayStats(status);
  if (!body || typeof body !== "object") return out;
  const o = body as Record<string, unknown>;
  out.uptimeSeconds = asCount(o.uptime_seconds);
  out.requestCount = asCount(o.request_count);
  out.llmCallCount = asCount(o.llm_call_count);
  out.wsUp = o.ws_up === true;
  out.lastHeartbeat = typeof o.last_heartbeat === "string" ? o.last_heartbeat.trim() : "";
  return out;
}

import type { GatewayStatsProbe } from "./stats";
import type { HealthKind } from "./health";

export const OVERVIEW_POLL_MS = 15000;

export type ChannelHealthCounts = {
  running: number;
  missing: number;
  failed: number;
  parked: number;
  stopped: number;
};

export function emptyChannelHealth(): ChannelHealthCounts {
  return { running: 0, missing: 0, failed: 0, parked: 0, stopped: 0 };
}

/** Count catalog health only. Ignores env names, last_error, and any extra fields. */
export function countChannelHealth(rows: Array<{ health?: string; missing?: boolean }>): ChannelHealthCounts {
  const out = emptyChannelHealth();
  for (const row of rows) {
    const h = (row.health || "").trim().toLowerCase();
    if (h === "running") out.running += 1;
    else if (h === "missing") out.missing += 1;
    else if (h === "failed") out.failed += 1;
    else if (h === "parked") out.parked += 1;
    else if (row.missing === true) out.missing += 1;
    else out.stopped += 1;
  }
  return out;
}

export function formatUptime(seconds: number): string {
  const s = Math.max(0, Math.trunc(Number.isFinite(seconds) ? seconds : 0));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${r}s`;
  return `${r}s`;
}

export function errorStatus(e: unknown): number {
  const s = e instanceof Error ? e.message : String(e);
  const m = s.match(/^(\d{3})\b/);
  return m ? Number(m[1]) : 0;
}

export function isUnauthorizedStatus(status: number): boolean {
  return status === 401 || status === 403;
}

export type OverviewSnapshot = {
  health: HealthKind;
  healthStatus: number;
  stats: GatewayStatsProbe;
  agents: number | null;
  sessions: number | null;
  channels: ChannelHealthCounts | null;
  cronJobs: number | null;
  errors: string[];
  kind: HealthKind;
  loadedAt: string | null;
  stale: boolean;
};

export function markOverviewStale(prev: OverviewSnapshot, nowIso: string): OverviewSnapshot {
  return {
    ...prev,
    stale: true,
    kind: prev.kind === "connected" ? "degraded" : prev.kind,
    loadedAt: prev.loadedAt || nowIso,
  };
}

export function canKeepOverviewStale(prev: OverviewSnapshot | null): boolean {
  if (!prev) return false;
  if (prev.stale && prev.agents == null && prev.sessions == null && prev.channels == null && prev.stats.status !== 200) {
    return false;
  }
  return Boolean(prev.loadedAt) || prev.stats.status === 200 || prev.agents != null || prev.sessions != null || prev.channels != null;
}

export function deriveOverviewKind(input: {
  health: HealthKind;
  statsStatus: number;
  agents: number | null;
  sessions: number | null;
  channels: ChannelHealthCounts | null;
  errors: string[];
}): HealthKind {
  if (input.health === "unauthorized") return "unauthorized";
  if (input.health === "offline") return "offline";
  if (input.health === "degraded") return "degraded";
  const listsMissing = input.agents == null && input.sessions == null && input.channels == null;
  if (listsMissing && isUnauthorizedStatus(input.statsStatus)) return "unauthorized";
  if (input.statsStatus !== 200) return "degraded";
  if (input.agents == null || input.sessions == null || input.channels == null || input.errors.length) return "degraded";
  return "connected";
}



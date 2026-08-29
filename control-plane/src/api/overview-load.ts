import { jsonFetch, probeHealthz, probeStats } from "./client";
import { healthKind } from "./health";
import { formatPublicError } from "./public-error";
import {
  countChannelHealth,
  deriveOverviewKind,
  errorStatus,
  isUnauthorizedStatus,
  type ChannelHealthCounts,
  type OverviewSnapshot,
} from "./overview";

const LIST_TIMEOUT_MS = 5000;

function isAbort(e: unknown): boolean {
  return typeof e === "object" && e != null && (e as { name?: string }).name === "AbortError";
}

function failedList(label: string, reason: unknown, errors: string[]): null {
  if (isAbort(reason)) return null;
  const st = errorStatus(reason);
  if (!isUnauthorizedStatus(st)) errors.push(`${label}: ${formatPublicError(reason)}`);
  return null;
}

function listSignal(parent?: AbortSignal): { signal: AbortSignal; stop: () => void } {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), LIST_TIMEOUT_MS);
  const onAbort = () => ctrl.abort();
  if (parent) {
    if (parent.aborted) ctrl.abort();
    else parent.addEventListener("abort", onAbort, { once: true });
  }
  return {
    signal: ctrl.signal,
    stop: () => {
      clearTimeout(timer);
      if (parent) parent.removeEventListener("abort", onAbort);
    },
  };
}

function emptyLists(health: OverviewSnapshot["health"], hzStatus: number, stats: OverviewSnapshot["stats"]): OverviewSnapshot {
  return {
    health,
    healthStatus: hzStatus,
    stats,
    agents: null,
    sessions: null,
    channels: null,
    cronJobs: null,
    errors: [],
    kind: deriveOverviewKind({
      health,
      statsStatus: stats.status,
      agents: null,
      sessions: null,
      channels: null,
      errors: [],
    }),
  };
}

export async function loadOverview(signal?: AbortSignal): Promise<OverviewSnapshot> {
  const [hz, stats] = await Promise.all([probeHealthz(signal), probeStats(signal)]);
  const health = healthKind(hz.status, hz.ok);
  if (health === "offline" || health === "unauthorized") {
    return emptyLists(health, hz.status, stats);
  }

  const errors: string[] = [];
  const bound = listSignal(signal);
  try {
    const [agentsRes, sessionsRes, channelsRes, cronRes] = await Promise.allSettled([
      jsonFetch<{ agents?: unknown[] }>("/api/agents", { signal: bound.signal }),
      jsonFetch<{ sessions?: unknown[] }>("/api/sessions", { signal: bound.signal }),
      jsonFetch<{ channels?: Array<{ health?: string; missing?: boolean }> }>("/api/channels", { signal: bound.signal }),
      jsonFetch<{ jobs?: unknown[] }>("/api/cron", { signal: bound.signal }),
    ]);

    const agents =
      agentsRes.status === "fulfilled" ? (agentsRes.value.agents ?? []).length : failedList("agents", agentsRes.reason, errors);
    const sessions =
      sessionsRes.status === "fulfilled"
        ? (sessionsRes.value.sessions ?? []).length
        : failedList("sessions", sessionsRes.reason, errors);

    let channels: ChannelHealthCounts | null = null;
    if (channelsRes.status === "fulfilled") {
      channels = countChannelHealth(
        (channelsRes.value.channels ?? []).map((c) => ({
          health: typeof c.health === "string" ? c.health : "",
          missing: c.missing === true,
        })),
      );
    } else {
      failedList("channels", channelsRes.reason, errors);
    }

    const cronJobs = cronRes.status === "fulfilled" ? (cronRes.value.jobs ?? []).length : null;

    return {
      health,
      healthStatus: hz.status,
      stats,
      agents,
      sessions,
      channels,
      cronJobs,
      errors,
      kind: deriveOverviewKind({
        health,
        statsStatus: stats.status,
        agents,
        sessions,
        channels,
        errors,
      }),
    };
  } finally {
    bound.stop();
  }
}

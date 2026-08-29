import { api, probeHealthz, probeStats } from "./client";
import { channelsApi } from "./channels";
import { cronApi } from "./cron";
import { healthKind } from "./health";
import {
  countChannelHealth,
  deriveOverviewKind,
  errorStatus,
  isUnauthorizedStatus,
  type ChannelHealthCounts,
  type OverviewSnapshot,
} from "./overview";

function publicFail(e: unknown): string {
  let s = e instanceof Error ? e.message : String(e);
  s = s
    .replace(/Bearer\s+\S+/gi, "Bearer [redacted]")
    .replace(/\b(token|secret|bot_token|api[_-]?key)\b\s*[:=]\s*\S+/gi, "$1=[redacted]");
  if (s.length > 400) s = `${s.slice(0, 400)}…`;
  return s;
}

function failedList(label: string, reason: unknown, errors: string[]): null {
  const st = errorStatus(reason);
  if (!isUnauthorizedStatus(st)) errors.push(`${label}: ${publicFail(reason)}`);
  return null;
}

export async function loadOverview(signal?: AbortSignal): Promise<OverviewSnapshot> {
  const [hz, stats] = await Promise.all([probeHealthz(signal), probeStats(signal)]);
  const health = healthKind(hz.status, hz.ok);
  const errors: string[] = [];

  const [agentsRes, sessionsRes, channelsRes, cronRes] = await Promise.allSettled([
    api.listAgents(),
    api.listSessions(),
    channelsApi.list(),
    cronApi.list(),
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
}

export type HealthKind = "connected" | "degraded" | "offline" | "unauthorized";

/** Map a GET /healthz probe to chrome state. status<=0 is network/timeout. */
export function healthKind(status: number, ok: boolean): HealthKind {
  if (status === 401 || status === 403) return "unauthorized";
  if (status <= 0) return "offline";
  if (status === 200 && ok) return "connected";
  return "degraded";
}

export type HealthKind = "connected" | "degraded" | "offline" | "unauthorized";

/** Map a GET /healthz probe to chrome state. status<=0 is network/timeout. */
export function healthKind(status: number, ok: boolean): HealthKind {
  if (status === 401 || status === 403) return "unauthorized";
  if (status <= 0) return "offline";
  if (status === 200 && ok) return "connected";
  return "degraded";
}

/**
 * Reconcile public /healthz with authenticated /api/stats.
 * healthz 200 must not stay "connected" when stats is 401/403 or not JSON/200.
 */
export function combineGatewayKind(healthz: HealthKind, statsStatus: number): HealthKind {
  if (healthz === "unauthorized" || statsStatus === 401 || statsStatus === 403) return "unauthorized";
  if (healthz === "offline") return "offline";
  if (healthz === "connected" && statsStatus === 200) return "connected";
  return "degraded";
}

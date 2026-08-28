import { useEffect, useState } from "react";
import { probeHealthz, probeStats } from "../api/client";
import { healthKind, type HealthKind } from "../api/health";
import { useI18n, type MsgKey } from "../i18n";

const MIN_MS = 2000;
const MAX_MS = 15000;

const KIND_KEY: Record<HealthKind, MsgKey> = {
  connected: "chrome.gateway.connected",
  degraded: "chrome.gateway.degraded",
  offline: "chrome.gateway.offline",
  unauthorized: "chrome.gateway.unauthorized",
};

function kindColor(kind: HealthKind | null): string {
  if (kind === "connected") return "var(--green)";
  if (kind === "degraded" || kind === "unauthorized") return "var(--orange)";
  if (kind === "offline") return "var(--red)";
  return "var(--text-4)";
}

export function GatewayStatus() {
  const { t } = useI18n();
  const [kind, setKind] = useState<HealthKind | null>(null);
  const [lastHeartbeat, setLastHeartbeat] = useState("");

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let backoff = MIN_MS;
    const ac = new AbortController();

    const run = async () => {
      const [{ status, ok }, stats] = await Promise.all([probeHealthz(ac.signal), probeStats(ac.signal)]);
      if (cancelled) return;
      const next = healthKind(status, ok);
      setKind(next);
      setLastHeartbeat(stats.lastHeartbeat);
      const wait = next === "connected" ? MAX_MS : backoff;
      backoff = next === "connected" ? MIN_MS : Math.min(MAX_MS, backoff * 2);
      timer = setTimeout(run, wait);
    };
    void run();
    return () => {
      cancelled = true;
      ac.abort();
      if (timer) clearTimeout(timer);
    };
  }, []);

  const pulse = kind === "connected";
  let label = kind ? `${t("chrome.gateway")} · ${t(KIND_KEY[kind])}` : t("chrome.gateway");
  if (lastHeartbeat) {
    label = `${label} · ${t("chrome.gateway.heartbeat", { at: lastHeartbeat })}`;
  }

  return (
    <div
      className="z-gateway"
      data-health={kind ?? "checking"}
      data-last-heartbeat={lastHeartbeat || undefined}
      aria-live="polite"
      style={{
        display: "flex",
        alignItems: "center",
        gap: 7,
        border: "1px solid var(--border)",
        borderRadius: 999,
        padding: "5px 12px",
        fontSize: 12,
        fontWeight: 500,
        color: "var(--text-2)",
      }}
    >
      <span
        data-motion={pulse ? "pulse" : undefined}
        style={{
          width: 7,
          height: 7,
          borderRadius: "50%",
          background: kindColor(kind),
          flex: "none",
          animation: pulse ? "zPulse 1.8s linear infinite" : "none",
        }}
      />
      {label}
    </div>
  );
}

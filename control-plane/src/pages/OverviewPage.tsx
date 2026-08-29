import { useEffect, useState } from "react";
import { loadOverview } from "../api/overview-load";
import {
  OVERVIEW_POLL_MS,
  formatUptime,
  type ChannelHealthCounts,
  type OverviewSnapshot,
} from "../api/overview";
import { healthKind, type HealthKind } from "../api/health";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { KpiCard } from "../ui/KpiCard";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine } from "../ui/StatusLine";
import { CrmMetricsPage } from "./CrmMetrics";

const KIND_KEY: Record<HealthKind, MsgKey> = {
  connected: "chrome.gateway.connected",
  degraded: "chrome.gateway.degraded",
  offline: "chrome.gateway.offline",
  unauthorized: "chrome.gateway.unauthorized",
};

function kindTone(kind: HealthKind | null): "positive" | "warning" | "critical" | "neutral" {
  if (kind === "connected") return "positive";
  if (kind === "degraded" || kind === "unauthorized") return "warning";
  if (kind === "offline") return "critical";
  return "neutral";
}

function fmt(n: number | null | undefined, locale: string): string {
  if (n == null) return "—";
  return n.toLocaleString(locale === "en" ? "en-US" : "vi-VN");
}

function emptySnap(): OverviewSnapshot {
  return {
    health: "offline",
    healthStatus: 0,
    stats: { status: 0, uptimeSeconds: 0, requestCount: 0, llmCallCount: 0, wsUp: false, lastHeartbeat: "" },
    agents: null,
    sessions: null,
    channels: null,
    cronJobs: null,
    errors: [],
    kind: "offline",
  };
}

function ChannelBreakdown({ counts, t }: { counts: ChannelHealthCounts; t: (k: MsgKey, v?: Record<string, string | number>) => string }) {
  const rows: { key: keyof ChannelHealthCounts; tone: "positive" | "warning" | "critical" | "neutral"; label: MsgKey }[] = [
    { key: "running", tone: "positive", label: "overview.channels.running" },
    { key: "missing", tone: "warning", label: "overview.channels.missing" },
    { key: "failed", tone: "critical", label: "overview.channels.failed" },
    { key: "parked", tone: "warning", label: "overview.channels.parked" },
  ];
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
      {rows.map((r) => (
        <Badge key={r.key} tone={r.tone} data-channel-health={r.key}>
          {t(r.label, { n: counts[r.key] })}
        </Badge>
      ))}
    </div>
  );
}

export function OverviewPage() {
  const { t, locale } = useI18n();
  const [snap, setSnap] = useState<OverviewSnapshot | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let inFlight = false;
    const ac = new AbortController();

    const run = async () => {
      if (inFlight) {
        timer = setTimeout(run, OVERVIEW_POLL_MS);
        return;
      }
      inFlight = true;
      try {
        const next = await loadOverview(ac.signal);
        if (!cancelled) setSnap(next);
      } catch {
        if (!cancelled) {
          setSnap((prev) => {
            const fallback = prev ?? emptySnap();
            return { ...fallback, kind: healthKind(0, false), health: "offline", errors: [t("overview.offline")] };
          });
        }
      } finally {
        inFlight = false;
        if (!cancelled) {
          setLoading(false);
          timer = setTimeout(run, OVERVIEW_POLL_MS);
        }
      }
    };
    void run();
    return () => {
      cancelled = true;
      ac.abort();
      if (timer) clearTimeout(timer);
    };
  }, [t]);

  async function refresh() {
    setLoading(true);
    try {
      setSnap(await loadOverview());
    } finally {
      setLoading(false);
    }
  }

  const kind = snap?.kind ?? null;
  const stats = snap?.stats;
  const empty =
    kind === "connected" && snap?.agents === 0 && snap?.sessions === 0 && (snap.channels == null || snap.channels.running === 0);

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }} data-overview={kind ?? "loading"}>
      <SectionHeader
        icon="gauge"
        title={t("overview.title")}
        description={t("overview.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void refresh()} disabled={loading}>
            {t("common.refresh")}
          </Button>
        }
      />
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <Badge tone={kindTone(kind)} data-overview-health={kind ?? "checking"}>
          {kind == null ? t("common.loading") : `${t("overview.gateway")} · ${t(KIND_KEY[kind])}`}
        </Badge>
      </div>
      {loading && !snap ? <StatusLine kind="loading" /> : null}
      {kind === "unauthorized" ? <EmptyState>{t("overview.unauthorized")}</EmptyState> : null}
      {kind === "offline" ? <EmptyState>{t("overview.offline")}</EmptyState> : null}
      {kind === "degraded" ? (
        <p role="status" style={{ color: "var(--orange)", fontSize: 12.5, margin: 0 }}>
          {t("overview.degraded")}
        </p>
      ) : null}
      {snap?.errors.length ? <StatusLine kind="error">{snap.errors.join(" · ")}</StatusLine> : null}

      {kind !== "unauthorized" && kind !== "offline" && snap ? (
        <>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12, minWidth: 0 }}>
            <KpiCard
              data-kpi="gateway"
              label={t("overview.gateway")}
              value={t(KIND_KEY[kind ?? "degraded"])}
              icon="pulse"
              tint={kind === "connected" ? "var(--green)" : "var(--orange)"}
              tintBg={kind === "connected" ? "var(--green-bg)" : "var(--warn-bg)"}
            />
            <KpiCard
              data-kpi="uptime"
              label={t("overview.uptime")}
              value={stats?.status === 200 ? formatUptime(stats.uptimeSeconds) : "—"}
              icon="clock"
              tint="var(--accent)"
              tintBg="var(--accent-soft)"
            />
            <KpiCard
              data-kpi="requests"
              label={t("overview.requests")}
              value={stats?.status === 200 ? fmt(stats.requestCount, locale) : "—"}
              icon="inbox"
              tint="var(--accent)"
              tintBg="var(--accent-soft)"
            />
            <KpiCard
              data-kpi="llm"
              label={t("overview.llm")}
              value={stats?.status === 200 ? fmt(stats.llmCallCount, locale) : "—"}
              icon="bolt"
              tint="var(--accent)"
              tintBg="var(--accent-soft)"
            />
            <KpiCard
              data-kpi="ws"
              label={t("overview.ws")}
              value={stats?.status === 200 ? (stats.wsUp ? t("overview.ws.up") : t("overview.ws.down")) : "—"}
              icon="hook"
              tint={stats?.status === 200 && stats.wsUp ? "var(--green)" : "var(--text-3)"}
              tintBg={stats?.status === 200 && stats.wsUp ? "var(--green-bg)" : "var(--surface-2)"}
            />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 12, minWidth: 0 }}>
            <Card data-card="agents">
              <CardHeader icon="bolt" title={t("overview.agents")} meta={fmt(snap.agents, locale)} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)" }}>
                {snap.agents == null ? t("overview.partial") : t("overview.agents.meta", { n: snap.agents })}
              </div>
            </Card>
            <Card data-card="sessions">
              <CardHeader icon="list" title={t("overview.sessions")} meta={fmt(snap.sessions, locale)} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)" }}>
                {snap.sessions == null ? t("overview.partial") : t("overview.sessions.meta", { n: snap.sessions })}
              </div>
            </Card>
            <Card data-card="channels">
              <CardHeader icon="device" title={t("overview.channels")} />
              <div style={{ padding: "12px 16px" }}>
                {snap.channels ? (
                  <ChannelBreakdown counts={snap.channels} t={t} />
                ) : (
                  <span style={{ fontSize: 13, color: "var(--text-2)" }}>{t("overview.partial")}</span>
                )}
              </div>
            </Card>
            <Card data-card="heartbeat">
              <CardHeader icon="timer" title={t("overview.heartbeat")} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)", fontVariantNumeric: "tabular-nums" }}>
                {stats?.lastHeartbeat ? t("overview.heartbeat.at", { at: stats.lastHeartbeat }) : t("overview.heartbeat.none")}
              </div>
            </Card>
            {snap.cronJobs != null ? (
              <Card data-card="cron">
                <CardHeader icon="clock" title={t("overview.cron")} meta={fmt(snap.cronJobs, locale)} />
                <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)" }}>
                  {t("overview.cron.meta", { n: snap.cronJobs })}
                </div>
              </Card>
            ) : null}
          </div>
          {empty ? <EmptyState>{t("overview.empty")}</EmptyState> : null}
        </>
      ) : null}

      <CrmMetricsPage embedded />
    </div>
  );
}

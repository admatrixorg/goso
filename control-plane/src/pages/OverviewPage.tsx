import { useEffect, useRef, useState } from "react";
import { loadOverview } from "../api/overview-load";
import {
  OVERVIEW_POLL_MS,
  canKeepOverviewStale,
  formatUptime,
  markOverviewStale,
  type ChannelHealthCounts,
  type OverviewSnapshot,
} from "../api/overview";
import { formatStaleAt } from "../api/page-state";
import type { HealthKind } from "../api/health";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { KpiCard } from "../ui/KpiCard";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
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
    loadedAt: null,
    stale: false,
  };
}

function ChannelBreakdown({ counts, t }: { counts: ChannelHealthCounts; t: (k: MsgKey, v?: Record<string, string | number>) => string }) {
  const rows: { key: keyof ChannelHealthCounts; tone: "positive" | "warning" | "critical" | "neutral"; label: MsgKey }[] = [
    { key: "running", tone: "positive", label: "overview.channels.running" },
    { key: "missing", tone: "warning", label: "overview.channels.missing" },
    { key: "failed", tone: "critical", label: "overview.channels.failed" },
    { key: "parked", tone: "warning", label: "overview.channels.parked" },
    { key: "stopped", tone: "neutral", label: "overview.channels.stopped" },
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
  const seqRef = useRef(0);
  const acRef = useRef<AbortController | null>(null);

  const pull = async (): Promise<void> => {
    acRef.current?.abort();
    const ac = new AbortController();
    acRef.current = ac;
    const seq = ++seqRef.current;
    try {
      const next = await loadOverview(ac.signal);
      if (seq === seqRef.current && !ac.signal.aborted) setSnap(next);
    } catch {
      if (seq === seqRef.current && !ac.signal.aborted) {
        setSnap((prev) => {
          if (canKeepOverviewStale(prev)) return markOverviewStale(prev as OverviewSnapshot, new Date().toISOString());
          const fallback = emptySnap();
          return { ...fallback, errors: [t("overview.offline")] };
        });
      }
    } finally {
      if (seq === seqRef.current && !ac.signal.aborted) setLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const run = async () => {
      await pull();
      if (!cancelled) timer = setTimeout(run, OVERVIEW_POLL_MS);
    };
    void run();
    return () => {
      cancelled = true;
      acRef.current?.abort();
      if (timer) clearTimeout(timer);
    };
  }, []);

  async function refresh() {
    setLoading(true);
    await pull();
  }

  const kind = snap?.kind ?? null;
  const stats = snap?.stats;
  const blocking = kind === "unauthorized" || kind === "offline";
  const trueEmpty =
    kind === "connected" &&
    !snap?.stale &&
    snap?.agents === 0 &&
    snap?.sessions === 0 &&
    (snap.channels == null || snap.channels.running === 0);
  const pageKind = loading && !snap ? "loading" : snap?.stale ? "stale" : kind === "unauthorized" ? "permission" : kind === "offline" ? "error" : kind === "degraded" ? "error" : "ready";
  const dash = t("overview.unavailable");
  const statsOk = stats?.status === 200 && !blocking;

  return (
    <PageChrome
      icon="gauge"
      title={t("overview.title")}
      description={t("overview.desc")}
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void refresh()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      data-overview={kind ?? "loading"}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <Badge tone={kindTone(kind)} data-overview-health={kind ?? "checking"}>
          {kind == null ? t("common.loading") : `${t("overview.gateway")} · ${t(KIND_KEY[kind])}`}
        </Badge>
        {snap?.stale ? (
          <Badge tone="warning" data-overview-stale="">
            {t("common.staleAt", { at: formatStaleAt(snap.loadedAt, locale) || "—" })}
          </Badge>
        ) : null}
      </div>
      <PageStatus
        kind={pageKind === "stale" ? "stale" : pageKind === "permission" ? "permission" : pageKind === "loading" ? "loading" : pageKind === "error" && kind === "offline" ? "error" : "ready"}
        errorText={kind === "offline" ? t("overview.offline") : kind === "unauthorized" ? t("overview.unauthorized") : undefined}
        staleAt={formatStaleAt(snap?.loadedAt, locale)}
        onReload={() => void refresh()}
      />
      {kind === "degraded" && !snap?.stale ? (
        <p role="status" style={{ color: "var(--orange)", fontSize: 12.5, margin: 0 }}>
          {t("overview.degraded")}
        </p>
      ) : null}
      {kind === "unauthorized" ? (
        <StatusLine kind="error">{t("overview.unauthorized")}</StatusLine>
      ) : null}
      {snap?.errors.length && kind !== "unauthorized" ? <StatusLine kind="error">{snap.errors.join(" · ")}</StatusLine> : null}

      {snap || !loading ? (
        <>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12, minWidth: 0 }}>
            <KpiCard
              data-kpi="gateway"
              label={t("overview.gateway")}
              value={kind ? t(KIND_KEY[kind]) : dash}
              icon="pulse"
              tint={kind === "connected" ? "var(--green)" : kind === "offline" ? "var(--red)" : "var(--orange)"}
              tintBg={kind === "connected" ? "var(--green-bg)" : kind === "offline" ? "var(--red-bg)" : "var(--warn-bg)"}
            />
            <KpiCard
              data-kpi="uptime"
              label={t("overview.uptime")}
              value={statsOk ? formatUptime(stats.uptimeSeconds) : dash}
              icon="clock"
              tint="var(--accent)"
              tintBg="var(--accent-soft)"
            />
            <KpiCard
              data-kpi="requests"
              label={t("overview.requests")}
              value={statsOk ? fmt(stats.requestCount, locale) : dash}
              icon="inbox"
              tint="var(--accent)"
              tintBg="var(--accent-soft)"
            />
            <KpiCard
              data-kpi="llm"
              label={t("overview.llm")}
              value={statsOk ? fmt(stats.llmCallCount, locale) : dash}
              icon="bolt"
              tint="var(--accent)"
              tintBg="var(--accent-soft)"
            />
            <KpiCard
              data-kpi="ws"
              label={t("overview.ws")}
              value={statsOk ? (stats.wsUp ? t("overview.ws.up") : t("overview.ws.down")) : dash}
              icon="hook"
              tint={statsOk && stats.wsUp ? "var(--green)" : "var(--text-3)"}
              tintBg={statsOk && stats.wsUp ? "var(--green-bg)" : "var(--surface-2)"}
            />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 12, minWidth: 0 }}>
            <Card data-card="agents">
              <CardHeader icon="bolt" title={t("overview.agents")} meta={snap?.agents == null || blocking ? dash : fmt(snap.agents, locale)} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)" }}>
                {snap?.agents == null || blocking ? t("overview.partial") : t("overview.agents.meta", { n: snap.agents })}
              </div>
            </Card>
            <Card data-card="sessions">
              <CardHeader icon="list" title={t("overview.sessions")} meta={snap?.sessions == null || blocking ? dash : fmt(snap.sessions, locale)} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)" }}>
                {snap?.sessions == null || blocking ? t("overview.partial") : t("overview.sessions.meta", { n: snap.sessions })}
              </div>
            </Card>
            <Card data-card="channels">
              <CardHeader icon="device" title={t("overview.channels")} />
              <div style={{ padding: "12px 16px" }}>
                {snap?.channels && !blocking ? (
                  <ChannelBreakdown counts={snap.channels} t={t} />
                ) : (
                  <span style={{ fontSize: 13, color: "var(--text-2)" }}>{t("overview.partial")}</span>
                )}
              </div>
            </Card>
            <Card data-card="heartbeat">
              <CardHeader icon="timer" title={t("overview.heartbeat")} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)", fontVariantNumeric: "tabular-nums" }}>
                {statsOk && stats.lastHeartbeat ? t("overview.heartbeat.at", { at: stats.lastHeartbeat }) : t("overview.heartbeat.none")}
              </div>
            </Card>
            <Card data-card="cron">
              <CardHeader icon="clock" title={t("overview.cron")} meta={snap?.cronJobs == null || blocking ? dash : fmt(snap.cronJobs, locale)} />
              <div style={{ padding: "12px 16px", fontSize: 13, color: "var(--text-2)" }}>
                {snap?.cronJobs == null || blocking
                  ? t("overview.partial")
                  : snap.cronJobs === 0
                    ? t("overview.cron.meta", { n: 0 })
                    : t("overview.cron.meta", { n: snap.cronJobs })}
              </div>
            </Card>
          </div>
          <Card data-card="unsupported">
            <CardHeader icon="eye" title={t("overview.unsupported")} />
            <ul style={{ margin: 0, padding: "12px 16px 16px 32px", fontSize: 13, color: "var(--text-2)", display: "flex", flexDirection: "column", gap: 6 }}>
              <li>{t("overview.usageUnavailable")}</li>
              <li>{t("overview.clientsUnavailable")}</li>
              <li>{t("overview.runtimesUnavailable")}</li>
              <li>{t("overview.recentUnavailable")}</li>
              <li>{t("overview.databaseUnavailable")}</li>
              <li>{t("overview.providersCountUnavailable")}</li>
              <li>{t("overview.toolsCountUnavailable")}</li>
            </ul>
          </Card>
          {trueEmpty ? <EmptyState data-overview-empty="">{t("overview.empty")}</EmptyState> : null}
        </>
      ) : null}

      <div data-overview-crm-extra="">
        <p style={{ fontSize: 12, color: "var(--text-3)", margin: 0 }}>{t("overview.crmExtra")}</p>
        <CrmMetricsPage embedded />
      </div>
    </PageChrome>
  );
}

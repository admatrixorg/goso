import { useEffect, useState } from "react";
import {
  asCrmError,
  crmBase,
  crmHealth,
  crmOrgId,
  crmUpstream,
  fetchCrmAdvisor,
  fetchCrmMetrics,
  type CrmAdvice,
  type CrmMetrics,
} from "../api/crm";
import { isPermissionError } from "../api/page-state";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { KpiCard } from "../ui/KpiCard";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine } from "../ui/StatusLine";

function fmt(n: number | undefined, locale: string): string {
  if (n == null) return "—";
  return n.toLocaleString(locale === "en" ? "en-US" : "vi-VN");
}

export function CrmMetricsPage({ embedded = false }: { embedded?: boolean }) {
  const { t, locale } = useI18n();
  const [org, setOrg] = useState(crmOrgId);
  const [online, setOnline] = useState<boolean | null>(null);
  const [metrics, setMetrics] = useState<CrmMetrics | null>(null);
  const [advice, setAdvice] = useState<CrmAdvice[]>([]);
  const [err, setErr] = useState("");
  const [perm, setPerm] = useState(false);
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    const h = await crmHealth();
    setOnline(h.online);
    if (!h.online) {
      setMetrics(null);
      setAdvice([]);
      setErr("");
      setPerm(false);
      setLoading(false);
      return;
    }
    const orgId = org.trim() || crmOrgId();
    const [m, a] = await Promise.allSettled([fetchCrmMetrics(orgId), fetchCrmAdvisor(orgId)]);
    const parts: string[] = [];
    let denied = false;
    if (m.status === "fulfilled") setMetrics(m.value);
    else {
      setMetrics(null);
      denied = denied || isPermissionError(m.reason);
      parts.push(`metrics: ${asCrmError(m.reason)}`);
    }
    if (a.status === "fulfilled") setAdvice(a.value);
    else {
      setAdvice([]);
      denied = denied || isPermissionError(a.reason);
      parts.push(`advisor: ${asCrmError(a.reason)}`);
    }
    setPerm(denied);
    setErr(parts.join(" · "));
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  const kpis = metrics
    ? [
        { label: t("crm.sent"), value: fmt(metrics.messagesSent, locale), icon: "msg" as const, tint: "var(--accent)", tintBg: "var(--accent-soft)" },
        { label: t("crm.received"), value: fmt(metrics.messagesReceived, locale), icon: "inbox" as const, tint: "var(--accent)", tintBg: "var(--accent-soft)" },
        {
          label: t("crm.unreplied"),
          value: fmt(metrics.unreplied, locale),
          icon: "flame" as const,
          tint: "var(--red)",
          tintBg: "var(--red-bg)",
          note: metrics.unreplied ? t("crm.needReply") : t("crm.noStuck"),
          noteTone: metrics.unreplied ? ("critical" as const) : ("neutral" as const),
        },
        { label: t("crm.avgResponse"), value: fmt(metrics.avgResponseTime, locale), icon: "hourglass" as const, tint: "var(--green)", tintBg: "var(--green-bg)" },
        { label: t("crm.kpi"), value: fmt(metrics.kpiCompletionRate, locale), unit: "%", icon: "flag" as const, tint: "var(--accent)", tintBg: "var(--accent-soft)" },
        { label: t("crm.revenue"), value: fmt(metrics.revenueMonth, locale), icon: "trend" as const, tint: "var(--green)", tintBg: "var(--green-bg)" },
      ]
    : [];

  const body = (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <Badge tone={online ? "positive" : online === false ? "critical" : "neutral"}>
          {online == null ? t("crm.checking") : online ? t("crm.online") : t("crm.offline")}
        </Badge>
        <span style={{ fontSize: 12, color: "var(--text-3)" }}>
          {crmBase()}
          {crmBase() === "/crm-api" ? ` → ${crmUpstream()}` : ""}
          {" · "}X-Org-ID
        </span>
        <input className="z-field" style={{ minWidth: 0, flex: 1 }} value={org} onChange={(e) => setOrg(e.target.value)} aria-label="CRM org id" />
      </div>
      {perm ? <StatusLine kind="error">{t("crm.permission")}</StatusLine> : null}
      {err && !perm ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      {online === false ? (
        <EmptyState>{t("crm.offlineEmpty")}</EmptyState>
      ) : perm ? (
        <EmptyState>{t("crm.permission")}</EmptyState>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12, minWidth: 0 }}>
          {kpis.map((k) => (
            <KpiCard key={k.label} {...k} />
          ))}
        </div>
      )}
      {metrics?.sampleDays != null ? (
        <p style={{ fontSize: 12, color: "var(--text-3)", fontStyle: "italic", margin: 0 }}>
          {t("crm.sample", { n: metrics.sampleDays, from: metrics.from ?? "—", to: metrics.to ?? "—" })}
          {metrics.orgId ? ` · org ${metrics.orgId}` : ""}
        </p>
      ) : null}
      <Card>
        <CardHeader icon="eye" title={t("crm.advisor")} meta={perm ? "—" : t("crm.adviceMeta", { n: advice.length })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.2 }}>{t("crm.col.kind")}</span>
          <span style={{ flex: 3.4 }}>{t("crm.col.summary")}</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>{t("crm.col.conf")}</span>
        </div>
        {advice.map((a, i) => (
          <div key={`${a.kind}-${i}`} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 1.2 }}>
              <Badge tone="accent">{a.kind}</Badge>
            </span>
            <span style={{ flex: 3.4 }}>{a.summary}</span>
            <span style={{ flex: 0.8, textAlign: "right", fontVariantNumeric: "tabular-nums", fontWeight: 600 }}>{a.confidence}</span>
          </div>
        ))}
        {!perm && !err && advice.length === 0 ? <EmptyState>{t("crm.emptyAdvice")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </>
  );

  if (embedded) {
    return (
      <Card data-overview-crm="">
        <CardHeader
          icon="gauge"
          title={t("crm.title")}
          meta={
            <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
              {t("common.refresh")}
            </Button>
          }
        />
        <div style={{ padding: "12px 16px 16px", display: "flex", flexDirection: "column", gap: 14 }}>
          <Badge tone="neutral">{t("crm.extra")}</Badge>
          <p style={{ fontSize: 12, color: "var(--text-3)", margin: 0 }}>{t("crm.desc")}</p>
          {body}
        </div>
      </Card>
    );
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="gauge"
        title={t("crm.title")}
        description={t("crm.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
            {t("common.refresh")}
          </Button>
        }
      />
      {body}
    </div>
  );
}

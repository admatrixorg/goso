import { useEffect, useMemo, useState } from "react";
import { asCrmError, crmBase, crmHealth, crmOrgId, crmUpstream, fetchCrmHeatmap, type HeatmapReport } from "../api/crm";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

function isoDay(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function defaultWindow(): { from: string; to: string } {
  const to = new Date();
  const from = new Date(to.getTime() - 13 * 86400000);
  return { from: isoDay(from), to: isoDay(to) };
}

function fmt(n: number, locale: string): string {
  return n.toLocaleString(locale === "en" ? "en-US" : "vi-VN");
}

export function HeatmapPage() {
  const { t, locale } = useI18n();
  const win = useMemo(defaultWindow, []);
  const [org, setOrg] = useState(crmOrgId);
  const [from, setFrom] = useState(win.from);
  const [to, setTo] = useState(win.to);
  const [online, setOnline] = useState<boolean | null>(null);
  const [report, setReport] = useState<HeatmapReport | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    const h = await crmHealth();
    setOnline(h.online);
    if (!h.online) {
      setReport(null);
      setErr("");
      setLoading(false);
      return;
    }
    try {
      const r = await fetchCrmHeatmap(org.trim() || crmOrgId(), from, to);
      setReport(r);
      setErr("");
    } catch (e) {
      setReport(null);
      setErr(asCrmError(e));
    }
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  const maxVol = Math.max(1, ...(report?.buckets ?? []).map((b) => b.messagesSent + b.messagesReceived));

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="report"
        title={t("heat.title")}
        description={t("heat.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
            {t("common.refresh")}
          </Button>
        }
      />
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <Badge tone={online ? "positive" : online === false ? "critical" : "neutral"}>
          {online == null ? t("crm.checking") : online ? t("crm.online") : t("crm.offline")}
        </Badge>
        <span style={{ fontSize: 12, color: "var(--text-3)" }}>
          {crmBase()}
          {crmBase() === "/crm-api" ? ` → ${crmUpstream()}` : ""}
          {" · "}
          {t("common.org")}
        </span>
        <input className="z-field" style={{ minWidth: 260 }} value={org} onChange={(e) => setOrg(e.target.value)} aria-label="CRM org id" />
        <input className="z-field" type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="from" />
        <input className="z-field" type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label="to" />
      </div>
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      {online === false ? (
        <EmptyState>{t("crm.offlineEmpty")}</EmptyState>
      ) : loading && !report ? null : !report || report.buckets.length === 0 ? (
        <EmptyState>{t("heat.empty")}</EmptyState>
      ) : (
        <>
          <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
            {report.buckets.map((b) => {
              const vol = b.messagesSent + b.messagesReceived;
              const tInt = vol / maxVol;
              return (
                <div
                  key={b.date}
                  title={`${b.date}: ${vol}`}
                  style={{
                    width: 28,
                    height: 28,
                    borderRadius: 6,
                    background: `color-mix(in srgb, var(--accent) ${Math.round(18 + tInt * 82)}%, var(--surface-2))`,
                    border: "1px solid var(--border)",
                  }}
                />
              );
            })}
          </div>
          <Card>
            <CardHeader icon="scatter" title={t("heat.title")} meta={t("heat.grain", { grain: report.grain ?? "day" })} />
            <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
              <span style={{ flex: 1.2 }}>{t("heat.col.date")}</span>
              <span style={{ flex: 0.8, textAlign: "right" }}>{t("heat.col.sent")}</span>
              <span style={{ flex: 0.8, textAlign: "right" }}>{t("heat.col.received")}</span>
              <span style={{ flex: 0.9, textAlign: "right" }}>{t("heat.col.unreplied")}</span>
              <span style={{ flex: 0.8, textAlign: "right" }}>{t("heat.col.kpi")}</span>
              <span style={{ flex: 1, textAlign: "right" }}>{t("heat.col.revenue")}</span>
            </div>
            {report.buckets.map((b) => (
              <div key={b.date} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", fontVariantNumeric: "tabular-nums" }}>
                <span style={{ flex: 1.2, fontWeight: 600 }}>{b.date}</span>
                <span style={{ flex: 0.8, textAlign: "right" }}>{fmt(b.messagesSent, locale)}</span>
                <span style={{ flex: 0.8, textAlign: "right" }}>{fmt(b.messagesReceived, locale)}</span>
                <span style={{ flex: 0.9, textAlign: "right" }}>{fmt(b.unreplied, locale)}</span>
                <span style={{ flex: 0.8, textAlign: "right" }}>{fmt(b.kpiCompletionRate, locale)}</span>
                <span style={{ flex: 1, textAlign: "right" }}>{fmt(b.revenue, locale)}</span>
              </div>
            ))}
          </Card>
        </>
      )}
    </div>
  );
}

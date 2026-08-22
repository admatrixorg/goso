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
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { KpiCard } from "../ui/KpiCard";
import { SectionHeader } from "../ui/SectionHeader";

function fmt(n: number | undefined): string {
  if (n == null) return "—";
  return n.toLocaleString("vi-VN");
}

export function CrmMetricsPage() {
  const [org, setOrg] = useState(crmOrgId);
  const [online, setOnline] = useState<boolean | null>(null);
  const [metrics, setMetrics] = useState<CrmMetrics | null>(null);
  const [advice, setAdvice] = useState<CrmAdvice[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    const h = await crmHealth();
    setOnline(h.online);
    if (!h.online) {
      setMetrics(null);
      setAdvice([]);
      setErr("");
      setLoading(false);
      return;
    }
    const orgId = org.trim() || crmOrgId();
    const [m, a] = await Promise.allSettled([fetchCrmMetrics(orgId), fetchCrmAdvisor(orgId)]);
    const parts: string[] = [];
    if (m.status === "fulfilled") setMetrics(m.value);
    else {
      setMetrics(null);
      parts.push(`metrics: ${asCrmError(m.reason)}`);
    }
    if (a.status === "fulfilled") setAdvice(a.value);
    else {
      setAdvice([]);
      parts.push(`advisor: ${asCrmError(a.reason)}`);
    }
    setErr(parts.join(" · "));
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  const kpis = metrics
    ? [
        { label: "Tin gửi", value: fmt(metrics.messagesSent), icon: "msg" as const, tint: "var(--accent)", tintBg: "var(--accent-soft)" },
        { label: "Tin nhận", value: fmt(metrics.messagesReceived), icon: "inbox" as const, tint: "var(--accent)", tintBg: "var(--accent-soft)" },
        {
          label: "Chưa rep",
          value: fmt(metrics.unreplied),
          icon: "flame" as const,
          tint: "var(--red)",
          tintBg: "var(--red-bg)",
          note: metrics.unreplied ? "Cần trả lời" : "Không có lead bị kẹt.",
          noteTone: metrics.unreplied ? ("critical" as const) : ("neutral" as const),
        },
        { label: "Phản hồi TB", value: fmt(metrics.avgResponseTime), icon: "hourglass" as const, tint: "var(--green)", tintBg: "var(--green-bg)" },
        { label: "KPI hoàn thành", value: fmt(metrics.kpiCompletionRate), unit: "%", icon: "flag" as const, tint: "var(--accent)", tintBg: "var(--accent-soft)" },
        { label: "Doanh thu tháng", value: fmt(metrics.revenueMonth), icon: "trend" as const, tint: "var(--green)", tintBg: "var(--green-bg)" },
      ]
    : [];

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="gauge"
        title="Tổng quan"
        description="Ai đang làm tốt, ai đang kẹt — đo công bằng theo nhịp thật của từng org. KPI lấy live từ goso-crm HTTP, không import Go."
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
            Làm mới
          </Button>
        }
      />
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <Badge tone={online ? "positive" : online === false ? "critical" : "neutral"}>
          {online == null ? "đang kiểm tra…" : online ? "goso-crm online" : "goso-crm offline"}
        </Badge>
        <span style={{ fontSize: 12, color: "var(--text-3)" }}>
          {crmBase()}
          {crmBase() === "/crm-api" ? ` → ${crmUpstream()}` : ""}
          {" · "}X-Org-ID
        </span>
        <input className="z-field" style={{ minWidth: 280 }} value={org} onChange={(e) => setOrg(e.target.value)} aria-label="CRM org id" />
      </div>
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      {online === false ? (
        <EmptyState>goso-crm offline — metrics unavailable.</EmptyState>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12 }}>
          {kpis.map((k) => (
            <KpiCard key={k.label} {...k} />
          ))}
        </div>
      )}
      {metrics?.sampleDays != null ? (
        <p style={{ fontSize: 12, color: "var(--text-3)", fontStyle: "italic", margin: 0 }}>
          {metrics.sampleDays} ngày mẫu · cửa sổ {metrics.from ?? "—"} → {metrics.to ?? "—"}
          {metrics.orgId ? ` · org ${metrics.orgId}` : ""}
        </p>
      ) : null}
      <Card>
        <CardHeader icon="eye" title="Advisor" meta={`${advice.length} gợi ý`} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.2 }}>LOẠI</span>
          <span style={{ flex: 3.4 }}>TÓM TẮT</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>CONF</span>
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
        {advice.length === 0 ? <EmptyState>Chưa có advice.</EmptyState> : null}
      </Card>
    </div>
  );
}

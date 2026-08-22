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

function fmt(n: number | undefined): string {
  if (n == null) return "—";
  return n.toLocaleString();
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

  const rows: { key: string; value: string }[] = metrics
    ? [
        { key: "messagesSent", value: fmt(metrics.messagesSent) },
        { key: "messagesReceived", value: fmt(metrics.messagesReceived) },
        { key: "unreplied", value: fmt(metrics.unreplied) },
        { key: "avgResponseTime", value: fmt(metrics.avgResponseTime) },
        { key: "kpiCompletionRate", value: fmt(metrics.kpiCompletionRate) },
        { key: "revenueMonth", value: fmt(metrics.revenueMonth) },
        ...(metrics.sampleDays != null ? [{ key: "sampleDays", value: fmt(metrics.sampleDays) }] : []),
      ]
    : [];

  return (
    <section>
      <h2>CRM metrics</h2>
      <p style={{ color: "#666", marginTop: 0 }}>
        Live KPI from goso-crm HTTP (
        <code>{crmBase()}</code>
        {crmBase() === "/crm-api" ? (
          <>
            {" "}
            → <code>{crmUpstream()}</code>
          </>
        ) : null}
        ). Header <code>X-Org-ID</code>. Empty/zero metrics are valid.
      </p>
      <p>
        <span
          style={{
            display: "inline-block",
            padding: "2px 8px",
            borderRadius: 4,
            fontWeight: 600,
            background: online == null ? "#e5e7eb" : online ? "#d1fae5" : "#fee2e2",
            color: online == null ? "#374151" : online ? "#065f46" : "#991b1b",
          }}
        >
          {online == null ? "checking…" : online ? "goso-crm online" : "goso-crm offline"}
        </span>
        {loading ? <small style={{ marginLeft: 8, color: "#666" }}>loading…</small> : null}
      </p>
      {err && <p style={{ color: "crimson" }}>{err}</p>}
      <div style={{ display: "flex", gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
        <input
          placeholder="X-Org-ID"
          value={org}
          onChange={(e) => setOrg(e.target.value)}
          style={{ minWidth: 280 }}
          aria-label="CRM org id"
        />
        <button onClick={() => void load()} disabled={loading}>
          Refresh
        </button>
      </div>
      <table style={{ width: "100%", borderCollapse: "collapse", marginBottom: 16 }}>
        <thead>
          <tr>
            <th align="left">kpi</th>
            <th align="left">value</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.key}>
              <td>
                <code>{r.key}</code>
              </td>
              <td>{r.value}</td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={2} style={{ color: "#666" }}>
                {online === false ? "(goso-crm offline — metrics unavailable)" : "(no metrics yet)"}
              </td>
            </tr>
          )}
        </tbody>
      </table>
      {metrics?.from || metrics?.to ? (
        <p style={{ color: "#666", fontSize: 13 }}>
          window: <code>{metrics.from ?? "—"}</code> → <code>{metrics.to ?? "—"}</code>
          {metrics.orgId ? (
            <>
              {" "}
              · org <code>{metrics.orgId}</code>
            </>
          ) : null}
        </p>
      ) : null}
      <h3>Advisor</h3>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
        <thead>
          <tr>
            <th align="left">kind</th>
            <th align="left">summary</th>
            <th align="left">confidence</th>
          </tr>
        </thead>
        <tbody>
          {advice.map((a, i) => (
            <tr key={`${a.kind}-${i}`}>
              <td>
                <code>{a.kind}</code>
              </td>
              <td>{a.summary}</td>
              <td>{a.confidence}</td>
            </tr>
          ))}
          {advice.length === 0 && (
            <tr>
              <td colSpan={3} style={{ color: "#666" }}>
                (no advice)
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  );
}

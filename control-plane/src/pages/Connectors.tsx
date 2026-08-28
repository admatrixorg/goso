import { useEffect, useState } from "react";
import { api, type Agent, type Connector } from "../api/client";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function healthTone(h?: string): "positive" | "warning" | "critical" | "neutral" {
  const s = (h || "").toLowerCase();
  if (s.includes("ok") || s.includes("healthy") || s === "up") return "positive";
  if (s.includes("fail") || s.includes("error") || s.includes("down")) return "critical";
  if (!h) return "neutral";
  return "warning";
}

export function ConnectorsPage() {
  const { t } = useI18n();
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("zalocrm");
  const [transport, setTransport] = useState("http");
  const [endpoint, setEndpoint] = useState("http://127.0.0.1:8089");
  const [agentId, setAgentId] = useState("");
  const [linkName, setLinkName] = useState("");

  async function load() {
    try {
      const [c, a] = await Promise.all([api.listConnectors(), api.listAgents()]);
      setConnectors(c.connectors ?? []);
      setAgents(a.agents ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void load();
  }, []);

  async function create() {
    if (!name.trim()) {
      setErr(t("connectors.needName"));
      return;
    }
    if (!endpoint.trim()) {
      setErr(t("connectors.needEndpoint"));
      return;
    }
    try {
      await api.createConnector({
        name: name.trim(),
        transport,
        endpoint: endpoint.trim(),
        enabled: true,
      });
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function assign() {
    if (!agentId) {
      setErr(t("connectors.needAgent"));
      return;
    }
    if (!linkName) {
      setErr(t("connectors.needConnector"));
      return;
    }
    try {
      await api.linkAgentConnector(agentId, linkName);
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="hook"
        title={t("connectors.title")}
        description={t("connectors.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void create()}>
              {t("connectors.register")}
            </Button>
          </>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <Card style={{ padding: 14, display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input className="z-field" placeholder="name" value={name} onChange={(e) => setName(e.target.value)} />
        <select className="z-field" value={transport} onChange={(e) => setTransport(e.target.value)}>
          <option value="http">http</option>
          <option value="mcp-http">mcp-http</option>
          <option value="mcp-stdio">mcp-stdio</option>
        </select>
        <input
          className="z-field"
          placeholder="endpoint"
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          style={{ minWidth: 0, flex: 1 }}
        />
      </Card>
      <Card>
        <CardHeader icon="device" title={t("connectors.list")} meta={t("connectors.meta", { n: connectors.length })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.2 }}>{t("connectors.col.name")}</span>
          <span style={{ flex: 1 }}>{t("connectors.col.transport")}</span>
          <span style={{ flex: 2.4 }}>{t("connectors.col.endpoint")}</span>
          <span style={{ flex: 1 }}>{t("connectors.col.health")}</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>{t("connectors.col.on")}</span>
        </div>
        {connectors.map((c) => (
          <div key={c.name} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 1.2, fontWeight: 600 }}>{c.name}</span>
            <span style={{ flex: 1, color: "var(--text-2)" }}>{c.transport}</span>
            <span style={{ flex: 2.4, color: "var(--text-3)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{c.endpoint}</span>
            <span style={{ flex: 1 }}>
              <Badge tone={healthTone(c.health)}>{c.health ?? "—"}</Badge>
            </span>
            <span style={{ flex: 0.8, textAlign: "right" }}>
              <Badge tone={c.enabled ? "positive" : "neutral"}>{c.enabled ? t("common.enabled") : t("common.disabled")}</Badge>
            </span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : connectors.length === 0 ? <EmptyState>{t("connectors.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="user-check" title={t("connectors.assign")} />
        <div style={{ padding: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
          <select className="z-field" value={agentId} onChange={(e) => setAgentId(e.target.value)}>
            <option value="">{t("connectors.pickAgent")}</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.display_name || a.agent_key}
              </option>
            ))}
          </select>
          <select className="z-field" value={linkName} onChange={(e) => setLinkName(e.target.value)}>
            <option value="">{t("connectors.pickConnector")}</option>
            {connectors.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
          <Button variant="primary" onClick={() => void assign()}>
            {t("connectors.assignBtn")}
          </Button>
        </div>
      </Card>
    </div>
  );
}

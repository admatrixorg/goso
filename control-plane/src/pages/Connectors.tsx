import { useEffect, useState } from "react";
import { api, type Agent, type Connector } from "../api/client";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

function healthTone(h?: string): "positive" | "warning" | "critical" | "neutral" {
  const s = (h || "").toLowerCase();
  if (s.includes("ok") || s.includes("healthy") || s === "up") return "positive";
  if (s.includes("fail") || s.includes("error") || s.includes("down")) return "critical";
  if (!h) return "neutral";
  return "warning";
}

export function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState("");
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
      setErr(String(e));
    }
  }
  useEffect(() => {
    void load();
  }, []);

  async function create() {
    if (!name.trim() || !endpoint.trim()) return;
    try {
      await api.createConnector({
        name: name.trim(),
        transport,
        endpoint: endpoint.trim(),
        enabled: true,
      });
      await load();
    } catch (e) {
      setErr(String(e));
    }
  }

  async function assign() {
    if (!agentId || !linkName) return;
    try {
      await api.linkAgentConnector(agentId, linkName);
      await load();
    } catch (e) {
      setErr(String(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="hook"
        title="Kết nối"
        description="Connector HTTP/MCP tới hệ thống ngoài (goso-crm). Gateway không chứa code ZaloCRM — chỉ gọi API."
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              Làm mới
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void create()}>
              Đăng ký
            </Button>
          </>
        }
      />
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
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
          style={{ minWidth: 280, flex: 1 }}
        />
      </Card>
      <Card>
        <CardHeader icon="device" title="Connector đã đăng ký" meta={`${connectors.length} kết nối`} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.2 }}>TÊN</span>
          <span style={{ flex: 1 }}>TRANSPORT</span>
          <span style={{ flex: 2.4 }}>ENDPOINT</span>
          <span style={{ flex: 1 }}>SỨC KHOẺ</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>BẬT</span>
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
              <Badge tone={c.enabled ? "positive" : "neutral"}>{c.enabled ? "bật" : "tắt"}</Badge>
            </span>
          </div>
        ))}
        {connectors.length === 0 ? <EmptyState>Chưa có connector.</EmptyState> : null}
      </Card>
      <Card>
        <CardHeader icon="user-check" title="Gán vào agent" />
        <div style={{ padding: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
          <select className="z-field" value={agentId} onChange={(e) => setAgentId(e.target.value)}>
            <option value="">agent…</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.display_name || a.agent_key}
              </option>
            ))}
          </select>
          <select className="z-field" value={linkName} onChange={(e) => setLinkName(e.target.value)}>
            <option value="">connector…</option>
            {connectors.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
          <Button variant="primary" onClick={() => void assign()}>
            Gán
          </Button>
        </div>
      </Card>
    </div>
  );
}

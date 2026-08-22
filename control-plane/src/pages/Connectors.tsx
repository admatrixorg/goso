import { useEffect, useState } from "react";
import { api, type Agent, type Connector } from "../api/client";

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
  useEffect(() => { void load(); }, []);

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
    } catch (e) { setErr(String(e)); }
  }

  async function assign() {
    if (!agentId || !linkName) return;
    try {
      await api.linkAgentConnector(agentId, linkName);
      await load();
    } catch (e) { setErr(String(e)); }
  }

  return (
    <section>
      <h2>Connectors</h2>
      {err && <p style={{ color: "crimson" }}>{err}</p>}
      <div style={{ display: "flex", gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
        <input placeholder="name" value={name} onChange={(e) => setName(e.target.value)} />
        <select value={transport} onChange={(e) => setTransport(e.target.value)}>
          <option value="http">http</option>
          <option value="mcp-http">mcp-http</option>
          <option value="mcp-stdio">mcp-stdio</option>
        </select>
        <input placeholder="endpoint" value={endpoint} onChange={(e) => setEndpoint(e.target.value)} style={{ minWidth: 240 }} />
        <button onClick={() => void create()}>Register</button>
        <button onClick={() => void load()}>Refresh</button>
      </div>
      <table style={{ width: "100%", borderCollapse: "collapse", marginBottom: 16 }}>
        <thead>
          <tr>
            <th align="left">name</th>
            <th align="left">transport</th>
            <th align="left">endpoint</th>
            <th align="left">health</th>
            <th align="left">enabled</th>
          </tr>
        </thead>
        <tbody>
          {connectors.map((c) => (
            <tr key={c.name}>
              <td><code>{c.name}</code></td>
              <td>{c.transport}</td>
              <td><small>{c.endpoint}</small></td>
              <td>{c.health ?? "—"}</td>
              <td>{c.enabled ? "yes" : "no"}</td>
            </tr>
          ))}
          {connectors.length === 0 && (
            <tr><td colSpan={5} style={{ color: "#666" }}>(no connectors)</td></tr>
          )}
        </tbody>
      </table>
      <h3>Assign to agent</h3>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <select value={agentId} onChange={(e) => setAgentId(e.target.value)}>
          <option value="">agent…</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>{a.display_name || a.agent_key} ({a.id})</option>
          ))}
        </select>
        <select value={linkName} onChange={(e) => setLinkName(e.target.value)}>
          <option value="">connector…</option>
          {connectors.map((c) => (
            <option key={c.name} value={c.name}>{c.name}</option>
          ))}
        </select>
        <button onClick={() => void assign()}>Assign</button>
      </div>
    </section>
  );
}

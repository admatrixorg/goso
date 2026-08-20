import { useEffect, useState } from "react";
import { api, type Agent } from "../api/client";

export function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState("");
  const [key, setKey] = useState("");
  const [name, setName] = useState("");

  async function load() {
    try {
      const j = await api.listAgents();
      setAgents(j.agents ?? []);
      setErr("");
    } catch (e) {
      setErr(String(e));
    }
  }
  useEffect(() => { void load(); }, []);

  async function create() {
    if (!key.trim()) return;
    try {
      await api.createAgent({ agent_key: key.trim(), display_name: name.trim() || key.trim() });
      setKey(""); setName(""); await load();
    } catch (e) { setErr(String(e)); }
  }

  return (
    <section>
      <h2>Agents</h2>
      {err && <p style={{ color: "crimson" }}>{err}</p>}
      <div style={{ display: "flex", gap: 8, marginBottom: 12 }}>
        <input placeholder="agent_key" value={key} onChange={(e) => setKey(e.target.value)} />
        <input placeholder="display_name" value={name} onChange={(e) => setName(e.target.value)} />
        <button onClick={() => void create()}>Create</button>
        <button onClick={() => void load()}>Refresh</button>
      </div>
      <ul>
        {agents.map((a) => (
          <li key={a.id}><code>{a.agent_key}</code> — {a.display_name} <small>({a.id})</small></li>
        ))}
        {agents.length === 0 && <li style={{ color: "#666" }}>(no agents)</li>}
      </ul>
    </section>
  );
}

import { useEffect, useState } from "react";
import { api, type Agent } from "../api/client";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

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
  useEffect(() => {
    void load();
  }, []);

  async function create() {
    if (!key.trim()) return;
    try {
      await api.createAgent({ agent_key: key.trim(), display_name: name.trim() || key.trim() });
      setKey("");
      setName("");
      await load();
    } catch (e) {
      setErr(String(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="bolt"
        title="Agent"
        description="Agent LLM của gateway — mỗi agent có model, session và tool connector riêng. Tạo xong thì gắn connector ở Kết nối."
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              Làm mới
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void create()}>
              Tạo agent
            </Button>
          </>
        }
      />
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder="agent_key" value={key} onChange={(e) => setKey(e.target.value)} />
        <input className="z-field" placeholder="display_name" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <Card>
        <CardHeader icon="user" title="Danh sách agent" meta={`${agents.length} agent`} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.4 }}>KEY</span>
          <span style={{ flex: 2 }}>TÊN</span>
          <span style={{ flex: 2 }}>ID</span>
          <span style={{ flex: 1.2 }}>MODEL</span>
        </div>
        {agents.map((a) => (
          <div
            key={a.id}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}
          >
            <span style={{ flex: 1.4, fontWeight: 600 }}>{a.agent_key}</span>
            <span style={{ flex: 2 }}>{a.display_name}</span>
            <span style={{ flex: 2, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{a.id}</span>
            <span style={{ flex: 1.2, color: "var(--text-2)" }}>{a.model || "—"}</span>
          </div>
        ))}
        {agents.length === 0 ? <EmptyState>Chưa có agent.</EmptyState> : null}
      </Card>
    </div>
  );
}

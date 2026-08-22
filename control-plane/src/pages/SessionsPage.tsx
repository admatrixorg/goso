import { useEffect, useState } from "react";
import { api, type Session } from "../api/client";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function SessionsPage({ onPick, compact }: { onPick: (id: string) => void; compact?: boolean }) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await api.listSessions();
      setSessions(j.sessions ?? []);
      setErr("");
    } catch (e) {
      setErr(String(e));
    }
  }
  useEffect(() => {
    void load();
  }, []);

  if (compact) {
    return (
      <div style={{ padding: 12, display: "flex", flexDirection: "column", gap: 8 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "4px 4px 8px" }}>
          <b style={{ fontSize: 13.5, fontWeight: 600, flex: 1 }}>Phiên</b>
          <Button icon="refresh" iconGesture variant="ghost" onClick={() => void load()} style={{ padding: "4px 8px" }}>
            Làm mới
          </Button>
        </div>
        {err ? <p style={{ color: "var(--red)", fontSize: 12, margin: 0 }}>{err}</p> : null}
        {sessions.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => onPick(s.id)}
            style={{
              display: "block",
              textAlign: "left",
              background: "var(--card)",
              border: "1px solid var(--border)",
              borderRadius: 11,
              padding: "10px 12px",
              transition: "background var(--dur-hover) var(--ease-standard)",
            }}
          >
            <div style={{ fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {s.label || s.id}
            </div>
            <div style={{ fontSize: 11.5, color: "var(--text-3)", marginTop: 3 }}>agent {s.agent_id}</div>
          </button>
        ))}
        {sessions.length === 0 ? <EmptyState>Chưa có phiên.</EmptyState> : null}
      </div>
    );
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="list"
        title="Phiên"
        description="Phiên chat gắn với một agent. Chọn phiên để vào Chat — không tạo session giả."
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            Làm mới
          </Button>
        }
      />
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      <Card>
        <CardHeader icon="msg" title="Phiên đang mở" meta={`${sessions.length} phiên`} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 2.4 }}>PHIÊN</span>
          <span style={{ flex: 2 }}>AGENT</span>
          <span style={{ flex: 1.2, textAlign: "right" }}></span>
        </div>
        {sessions.map((s) => (
          <div
            key={s.id}
            style={{
              display: "flex",
              alignItems: "center",
              padding: "11px 16px",
              fontSize: 12.5,
              borderBottom: "1px solid var(--border-soft)",
              cursor: "pointer",
            }}
            onClick={() => onPick(s.id)}
          >
            <span style={{ flex: 2.4, fontWeight: 600 }}>{s.label || s.id}</span>
            <span style={{ flex: 2, color: "var(--text-2)" }}>{s.agent_id}</span>
            <span style={{ flex: 1.2, textAlign: "right", color: "var(--accent)", fontWeight: 600, fontSize: 12 }}>Vào Chat</span>
          </div>
        ))}
        {sessions.length === 0 ? <EmptyState>Chưa có phiên.</EmptyState> : null}
      </Card>
    </div>
  );
}

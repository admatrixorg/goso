import { useState } from "react";
import { AgentsPage } from "./pages/AgentsPage";
import { SessionsPage } from "./pages/SessionsPage";
import { ChatPage } from "./pages/ChatPage";
import { ConnectorsPage } from "./pages/Connectors";
import { EventsPage } from "./pages/Events";
import { CrmMetricsPage } from "./pages/CrmMetrics";

const TABS = ["agents", "sessions", "chat", "connectors", "events", "crm"] as const;
type Tab = (typeof TABS)[number];

const TAB_LABEL: Record<Tab, string> = {
  agents: "agents",
  sessions: "sessions",
  chat: "chat",
  connectors: "connectors",
  events: "events",
  crm: "CRM metrics",
};

export default function App() {
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<Tab>("agents");

  return (
    <div style={{ fontFamily: "system-ui, sans-serif", maxWidth: 960, margin: "24px auto", padding: "0 16px" }}>
      <h1>GOSO Control Plane</h1>
      <p style={{ color: "#666" }}>Gateway: <code>{import.meta.env.VITE_GATEWAY_URL || "proxied via Vite (127.0.0.1:8080)"}</code> • Token: set <code>VITE_GOSO_ADMIN_TOKEN</code> or <code>localStorage.goso_token</code></p>
      <nav style={{ display: "flex", gap: 8, marginBottom: 16, flexWrap: "wrap" }}>
        {TABS.map((t) => (
          <button key={t} onClick={() => setTab(t)} style={{ fontWeight: tab === t ? 700 : 400 }}>{TAB_LABEL[t]}</button>
        ))}
      </nav>
      {tab === "agents" && <AgentsPage />}
      {tab === "sessions" && <SessionsPage onPick={(id) => { setSessionId(id); setTab("chat"); }} />}
      {tab === "chat" && (
        <>
          <SessionsPage onPick={setSessionId} />
          <ChatPage sessionId={sessionId} />
        </>
      )}
      {tab === "connectors" && <ConnectorsPage />}
      {tab === "events" && <EventsPage />}
      {tab === "crm" && <CrmMetricsPage />}
    </div>
  );
}

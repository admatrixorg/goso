import { useState } from "react";
import { AgentsPage } from "./pages/AgentsPage";
import { SessionsPage } from "./pages/SessionsPage";
import { ChatPage } from "./pages/ChatPage";

export default function App() {
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<"agents" | "sessions" | "chat">("agents");

  return (
    <div style={{ fontFamily: "system-ui, sans-serif", maxWidth: 860, margin: "24px auto", padding: "0 16px" }}>
      <h1>GOSO Control Plane</h1>
      <p style={{ color: "#666" }}>Gateway: <code>{(import.meta.env.VITE_GATEWAY_URL as string) || "proxied via Vite (127.0.0.1:8080)"}</code> • Token: set <code>VITE_GOSO_ADMIN_TOKEN</code> or <code>localStorage.goso_token</code></p>
      <nav style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        {(["agents", "sessions", "chat"] as const).map((t) => (
          <button key={t} onClick={() => setTab(t)} style={{ fontWeight: tab === t ? 700 : 400 }}>{t}</button>
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
    </div>
  );
}

import { useState } from "react";
import { AgentsPage } from "../../../control-plane/src/pages/AgentsPage";
import { ChatPage } from "../../../control-plane/src/pages/ChatPage";
import { SessionsPage } from "../../../control-plane/src/pages/SessionsPage";

export default function App() {
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<"agents" | "sessions" | "chat">("agents");

  return (
    <div style={{ fontFamily: "system-ui, sans-serif", maxWidth: 860, margin: "24px auto", padding: "0 16px" }}>
      <h1>GOSO Desktop</h1>
      <p style={{ color: "#666" }}>
        Local gateway + SQLite • Control Plane pages reused from <code>control-plane/</code>
      </p>
      <nav style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        {(["agents", "sessions", "chat"] as const).map((t) => (
          <button key={t} onClick={() => setTab(t)} style={{ fontWeight: tab === t ? 700 : 400 }}>
            {t}
          </button>
        ))}
      </nav>
      {tab === "agents" && <AgentsPage />}
      {tab === "sessions" && (
        <SessionsPage
          onPick={(id) => {
            setSessionId(id);
            setTab("chat");
          }}
        />
      )}
      {tab === "chat" && (
        <>
          <SessionsPage onPick={setSessionId} />
          <ChatPage sessionId={sessionId} />
        </>
      )}
    </div>
  );
}

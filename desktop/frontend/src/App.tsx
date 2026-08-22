import { useState } from "react";
import { AgentsPage } from "../../../control-plane/src/pages/AgentsPage";
import { ChatPage } from "../../../control-plane/src/pages/ChatPage";
import { SessionsPage } from "../../../control-plane/src/pages/SessionsPage";

export default function App() {
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<"agents" | "sessions" | "chat">("agents");

  return (
    <div style={{ minHeight: "100vh", background: "var(--bg)", color: "var(--text)" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          background: "var(--chrome)",
          borderBottom: "1px solid var(--border)",
          padding: "7px 16px",
        }}
      >
        <div
          style={{
            width: 26,
            height: 26,
            borderRadius: 8,
            background: "var(--accent)",
            color: "#fff",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontWeight: 600,
            fontSize: 14,
          }}
        >
          Z
        </div>
        <div style={{ fontWeight: 600, fontSize: 14 }}>GOSO Desktop</div>
        <nav style={{ display: "flex", gap: 4, marginLeft: 12 }}>
          {(["agents", "sessions", "chat"] as const).map((t) => {
            const on = tab === t;
            const label = t === "agents" ? "Agent" : t === "sessions" ? "Phiên" : "Chat";
            return (
              <button
                key={t}
                type="button"
                onClick={() => setTab(t)}
                style={{
                  background: "transparent",
                  border: "none",
                  padding: "8px 12px",
                  fontWeight: on ? 600 : 500,
                  color: on ? "var(--text)" : "var(--text-3)",
                  borderBottom: `2px solid ${on ? "var(--accent)" : "transparent"}`,
                }}
              >
                {label}
              </button>
            );
          })}
        </nav>
      </div>
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
        <div style={{ display: "flex", minHeight: "calc(100vh - 48px)" }}>
          <div style={{ width: 280, borderRight: "1px solid var(--border)" }}>
            <SessionsPage compact onPick={setSessionId} />
          </div>
          <div style={{ flex: 1 }}>
            <ChatPage sessionId={sessionId} />
          </div>
        </div>
      )}
    </div>
  );
}

import { useEffect, useState } from "react";
import { AgentsPage } from "./pages/AgentsPage";
import { SessionsPage } from "./pages/SessionsPage";
import { ChatPage } from "./pages/ChatPage";
import { ConnectorsPage } from "./pages/Connectors";
import { EventsPage } from "./pages/Events";
import { CrmMetricsPage } from "./pages/CrmMetrics";
import { Icon, type IconName } from "./ui/Icon";
import { Avatar } from "./ui/Avatar";

export type Tab = "crm" | "agents" | "sessions" | "chat" | "connectors" | "events";

const TOP: { id: Tab; label: string }[] = [
  { id: "crm", label: "Tổng quan" },
  { id: "agents", label: "Agent" },
  { id: "sessions", label: "Phiên" },
  { id: "chat", label: "Chat" },
  { id: "connectors", label: "Kết nối" },
  { id: "events", label: "Nhật ký" },
];

const SIDE: { group: string; items: { id: Tab; label: string; ic: IconName }[] }[] = [
  {
    group: "LÀM VIỆC",
    items: [
      { id: "crm", label: "Tổng quan", ic: "gauge" },
      { id: "agents", label: "Agent", ic: "bolt" },
      { id: "sessions", label: "Phiên", ic: "list" },
      { id: "chat", label: "Chat", ic: "msg" },
    ],
  },
  {
    group: "HỆ THỐNG",
    items: [
      { id: "connectors", label: "Kết nối", ic: "hook" },
      { id: "events", label: "Nhật ký", ic: "history" },
    ],
  },
];

function useTheme() {
  const [dark, setDark] = useState(() => document.body.classList.contains("dark"));
  useEffect(() => {
    document.body.classList.toggle("dark", dark);
  }, [dark]);
  return { dark, toggle: () => setDark((d) => !d) };
}

export default function App() {
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<Tab>("crm");
  const { dark, toggle } = useTheme();
  const [q, setQ] = useState("");

  function go(id: Tab) {
    setTab(id);
  }

  return (
    <div style={{ height: "100vh", minWidth: 1280, display: "flex", flexDirection: "column", overflow: "hidden", background: "var(--bg)" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 2,
          background: "var(--chrome)",
          borderBottom: "1px solid var(--border)",
          padding: "7px 16px",
          flex: "none",
          zIndex: 50,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginRight: 16 }}>
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
          <div style={{ fontWeight: 600, fontSize: 14, letterSpacing: "-.2px", color: "var(--text)" }}>ZAgent</div>
        </div>
        {TOP.map((it) => {
          const on = tab === it.id;
          return (
            <button
              key={it.id}
              type="button"
              onClick={() => go(it.id)}
              style={{
                background: "transparent",
                border: "none",
                padding: "8px 12px",
                fontSize: 13,
                fontWeight: on ? 600 : 500,
                color: on ? "var(--text)" : "var(--text-3)",
                borderBottom: `2px solid ${on ? "var(--accent)" : "transparent"}`,
                marginBottom: -8,
                borderRadius: 0,
                transition: "color var(--dur-control) var(--ease-standard), border-color var(--dur-control) var(--ease-standard)",
              }}
            >
              {it.label}
            </button>
          );
        })}
        <div style={{ flex: 1 }} />
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 7,
              border: "1px solid var(--border)",
              borderRadius: 999,
              padding: "5px 12px",
              fontSize: 12,
              fontWeight: 500,
              color: "var(--text-2)",
            }}
          >
            <span
              data-motion="pulse"
              style={{
                width: 7,
                height: 7,
                borderRadius: "50%",
                background: "var(--green)",
                flex: "none",
                animation: "zPulse 1.8s linear infinite",
              }}
            />
            Gateway
          </div>
          <div
            data-ig="search"
            style={{
              background: "var(--surface-2)",
              borderRadius: 8,
              padding: "6px 12px",
              color: "var(--text-4)",
              fontSize: 12.5,
              display: "flex",
              gap: 8,
              alignItems: "center",
              width: 170,
            }}
          >
            <Icon name="search" size={13} />
            <input
              className="z-field"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Tìm kiếm…"
              aria-label="Tìm kiếm"
              style={{ border: "none", background: "transparent", padding: 0, minHeight: 0, width: "100%" }}
            />
          </div>
          <button
            type="button"
            onClick={toggle}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              border: "1px solid var(--border)",
              borderRadius: 999,
              padding: "4px 12px",
              fontSize: 12,
              fontWeight: 500,
              color: "var(--text-2)",
              background: "var(--card)",
            }}
          >
            ◐ {dark ? "Tối" : "Sáng"}
          </button>
          <span data-ig="bell" style={{ position: "relative", color: "var(--text-2)", display: "flex", alignItems: "center" }}>
            <span data-ig-part="">
              <Icon name="bell" size={15} />
            </span>
          </span>
          <Avatar initials="G" size={27} />
        </div>
      </div>

      <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
        <div
          style={{
            width: 216,
            flex: "none",
            height: "100%",
            background: "var(--chrome)",
            borderRight: "1px solid var(--border)",
            display: "flex",
            flexDirection: "column",
            overflowY: "auto",
            padding: "12px 8px 8px",
          }}
        >
          <div
            data-ig="search"
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              background: "var(--card)",
              border: "1px solid var(--border)",
              borderRadius: 9,
              padding: "7px 10px",
              fontSize: 12.5,
              color: "var(--text-4)",
              minHeight: 34,
            }}
          >
            <span style={{ flex: 1 }}>Tìm nhanh</span>
            <span style={{ fontSize: 11, color: "var(--text-4)", fontWeight: 500 }}>⌘K</span>
          </div>
          {SIDE.map((g) => (
            <div key={g.group} style={{ marginTop: 14 }}>
              <div
                style={{
                  fontSize: 10,
                  fontWeight: 700,
                  letterSpacing: ".7px",
                  color: "var(--text-3)",
                  padding: "0 10px 6px",
                }}
              >
                {g.group}
              </div>
              {g.items.map((i) => {
                const on = tab === i.id;
                return (
                  <button
                    key={i.id}
                    type="button"
                    onClick={() => go(i.id)}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 9,
                      width: "100%",
                      minHeight: 34,
                      padding: "7px 10px",
                      borderRadius: 8,
                      fontSize: 13,
                      border: "none",
                      textAlign: "left",
                      background: on ? "var(--accent-soft)" : "transparent",
                      color: on ? "var(--accent)" : "var(--text-2)",
                      fontWeight: on ? 600 : 400,
                      transition: "background var(--dur-control) var(--ease-standard), color var(--dur-control) var(--ease-standard)",
                    }}
                  >
                    <Icon name={i.ic} size={15} />
                    <span style={{ flex: 1 }}>{i.label}</span>
                  </button>
                );
              })}
            </div>
          ))}
          <div style={{ flex: 1 }} />
          <div style={{ borderTop: "1px solid var(--border-soft)", paddingTop: 8 }}>
            <div
              data-ig="gear"
              style={{
                display: "flex",
                alignItems: "center",
                gap: 9,
                minHeight: 34,
                padding: "7px 10px",
                borderRadius: 8,
                fontSize: 13,
                color: "var(--text-2)",
              }}
            >
              <span data-ig-part="">
                <Icon name="gear" size={15} />
              </span>
              <span style={{ flex: 1 }}>Cài đặt</span>
            </div>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 9,
                minHeight: 34,
                padding: "7px 10px",
                borderRadius: 8,
                fontSize: 12.5,
                color: "var(--text-2)",
              }}
            >
              <span
                style={{
                  width: 19,
                  height: 19,
                  borderRadius: 5,
                  background: "var(--accent)",
                  color: "#fff",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  fontSize: 10,
                  fontWeight: 600,
                  flex: "none",
                }}
              >
                G
              </span>
              <span style={{ flex: 1 }}>GOSO</span>
            </div>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0, height: "100%", display: "flex", flexDirection: "column", overflowY: "auto", overflowX: "hidden" }}>
          {tab === "crm" && <CrmMetricsPage />}
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
            <div style={{ display: "flex", flex: 1, minHeight: 0, height: "100%" }}>
              <div style={{ width: 280, flex: "none", borderRight: "1px solid var(--border)", overflowY: "auto" }}>
                <SessionsPage
                  compact
                  onPick={(id) => {
                    setSessionId(id);
                  }}
                />
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <ChatPage sessionId={sessionId} />
              </div>
            </div>
          )}
          {tab === "connectors" && <ConnectorsPage />}
          {tab === "events" && <EventsPage />}
        </div>
      </div>
    </div>
  );
}

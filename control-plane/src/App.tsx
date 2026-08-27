import { useEffect, useState } from "react";
import { AgentsPage } from "./pages/AgentsPage";
import { SessionsPage } from "./pages/SessionsPage";
import { ChatPage } from "./pages/ChatPage";
import { ConnectorsPage } from "./pages/Connectors";
import { FunctionsPage } from "./pages/FunctionsPage";
import { EventsPage } from "./pages/Events";
import { CrmMetricsPage } from "./pages/CrmMetrics";
import { HomePage } from "./pages/HomePage";
import { MeetingsPage } from "./pages/MeetingsPage";
import { TasksPage } from "./pages/TasksPage";
import { FriendsPage } from "./pages/FriendsPage";
import { CalendarPage } from "./pages/CalendarPage";
import { GalleryPage } from "./pages/GalleryPage";
import { MarketingPage } from "./pages/MarketingPage";
import { SettingsPage } from "./pages/SettingsPage";
import { HeatmapPage } from "./pages/HeatmapPage";
import { TeamsPage } from "./pages/TeamsPage";
import { VaultPage } from "./pages/VaultPage";
import { MemoryPage } from "./pages/MemoryPage";
import { ProvidersPage } from "./pages/ProvidersPage";
import { ChannelsPage } from "./pages/ChannelsPage";
import { WebhooksPage } from "./pages/WebhooksPage";
import { TracesPage } from "./pages/TracesPage";
import { Icon, type IconName } from "./ui/Icon";
import { Avatar } from "./ui/Avatar";
import { isDemoMode } from "./demo/mode";
import { demoOverviewItems, demoTop, demoWorkExtra } from "./demo/nav";
import { useI18n, type Locale } from "./i18n";

const DEMO = isDemoMode();

export type Tab =
  | "home"
  | "tasks"
  | "meetings"
  | "crm"
  | "agents"
  | "sessions"
  | "chat"
  | "friends"
  | "calendar"
  | "gallery"
  | "marketing"
  | "heatmap"
  | "connectors"
  | "functions"
  | "events"
  | "teams"
  | "vault"
  | "memory"
  | "providers"
  | "channels"
  | "webhooks"
  | "traces"
  | "settings";

function liveTop(t: (k: "nav.overview" | "nav.chat" | "nav.connectors" | "nav.events") => string): { id: Tab; label: string }[] {
  return [
    { id: "crm", label: t("nav.overview") },
    { id: "chat", label: t("nav.chat") },
    { id: "connectors", label: t("nav.connectors") },
    { id: "events", label: t("nav.events") },
  ];
}

function liveSide(t: ReturnType<typeof useI18n>["t"]): { group: string; items: { id: Tab; label: string; ic: IconName }[] }[] {
  return [
    {
      group: t("nav.group.overview"),
      items: [
        { id: "crm", label: t("nav.overview"), ic: "gauge" },
        { id: "heatmap", label: t("nav.heatmap"), ic: "report" },
      ],
    },
    {
      group: t("nav.group.work"),
      items: [
        { id: "agents", label: t("nav.agents"), ic: "bolt" },
        { id: "sessions", label: t("nav.sessions"), ic: "list" },
        { id: "chat", label: t("nav.chat"), ic: "msg" },
        { id: "marketing", label: t("nav.marketing"), ic: "mega" },
        { id: "teams", label: t("nav.teams"), ic: "layers" },
        { id: "vault", label: t("nav.vault"), ic: "doc" },
        { id: "memory", label: t("nav.memory"), ic: "inbox" },
      ],
    },
    {
      group: t("nav.group.system"),
      items: [
        { id: "connectors", label: t("nav.connectors"), ic: "hook" },
        { id: "functions", label: t("nav.functions"), ic: "build" },
        { id: "events", label: t("nav.events"), ic: "history" },
        { id: "providers", label: t("nav.providers"), ic: "bolt" },
        { id: "channels", label: t("nav.channels"), ic: "device" },
        { id: "webhooks", label: t("nav.webhooks"), ic: "hook" },
        { id: "traces", label: t("nav.traces"), ic: "history" },
      ],
    },
  ];
}

function useTheme() {
  const [dark, setDark] = useState(() => document.body.classList.contains("dark"));
  useEffect(() => {
    document.body.classList.toggle("dark", dark);
  }, [dark]);
  return { dark, toggle: () => setDark((d) => !d) };
}

export default function App() {
  const { t, locale, setLocale } = useI18n();
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<Tab>(DEMO ? "home" : "crm");
  const { dark, toggle } = useTheme();
  const [q, setQ] = useState("");

  const top = DEMO ? [...demoTop(locale), ...liveTop(t)] : liveTop(t);
  const side = DEMO
    ? [
        {
          group: t("nav.group.overview"),
          items: [...demoOverviewItems(locale), { id: "heatmap" as const, label: t("nav.heatmap"), ic: "report" as const }],
        },
        {
          group: t("nav.group.work"),
          items: [
            { id: "agents" as const, label: t("nav.agents"), ic: "bolt" as const },
            { id: "sessions" as const, label: t("nav.sessions"), ic: "list" as const },
            { id: "chat" as const, label: t("nav.chat"), ic: "msg" as const },
            { id: "marketing" as const, label: t("nav.marketing"), ic: "mega" as const },
            { id: "teams" as const, label: t("nav.teams"), ic: "layers" as const },
            { id: "vault" as const, label: t("nav.vault"), ic: "doc" as const },
            { id: "memory" as const, label: t("nav.memory"), ic: "inbox" as const },
            ...demoWorkExtra(locale),
          ],
        },
        liveSide(t)[2],
      ]
    : liveSide(t);

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
        {top.map((it) => {
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
            {t("chrome.gateway")}
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
              placeholder={t("chrome.search")}
              aria-label={t("chrome.search")}
              style={{ border: "none", background: "transparent", padding: 0, minHeight: 0, width: "100%" }}
            />
          </div>
          <LangToggle locale={locale} setLocale={setLocale} />
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
            ◐ {dark ? t("chrome.dark") : t("chrome.light")}
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
            <span style={{ flex: 1 }}>{t("chrome.quickSearch")}</span>
            <span style={{ fontSize: 11, color: "var(--text-4)", fontWeight: 500 }}>⌘K</span>
          </div>
          {side.map((g) => {
            const items = g.items;
            if (!items.length) return null;
            return (
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
              {items.map((i) => {
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
            );
          })}
          <div style={{ flex: 1 }} />
          <div style={{ borderTop: "1px solid var(--border-soft)", paddingTop: 8 }}>
            <button
              type="button"
              data-ig="gear"
              onClick={() => go("settings")}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 9,
                width: "100%",
                minHeight: 34,
                padding: "7px 10px",
                borderRadius: 8,
                fontSize: 13,
                color: tab === "settings" ? "var(--accent)" : "var(--text-2)",
                background: tab === "settings" ? "var(--accent-soft)" : "transparent",
                border: "none",
                fontWeight: tab === "settings" ? 600 : 400,
                textAlign: "left",
              }}
            >
              <span data-ig-part="">
                <Icon name="gear" size={15} />
              </span>
              <span style={{ flex: 1 }}>{t("nav.settings")}</span>
            </button>
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
          {DEMO && tab === "home" && <HomePage onMeetings={() => go("meetings")} onChat={() => go("chat")} />}
          {DEMO && tab === "meetings" && <MeetingsPage />}
          {DEMO && tab === "tasks" && <TasksPage onChat={() => go("chat")} />}
          {tab === "crm" && <CrmMetricsPage />}
          {tab === "heatmap" && <HeatmapPage />}
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
          {DEMO && tab === "friends" && <FriendsPage />}
          {DEMO && tab === "calendar" && <CalendarPage />}
          {DEMO && tab === "gallery" && <GalleryPage />}
          {tab === "marketing" && <MarketingPage />}
          {tab === "teams" && <TeamsPage />}
          {tab === "vault" && <VaultPage />}
          {tab === "memory" && <MemoryPage />}
          {tab === "providers" && <ProvidersPage />}
          {tab === "channels" && <ChannelsPage />}
          {tab === "webhooks" && <WebhooksPage />}
          {tab === "traces" && <TracesPage />}
          {tab === "connectors" && <ConnectorsPage />}
          {tab === "functions" && <FunctionsPage />}
          {tab === "events" && <EventsPage />}
          {tab === "settings" && <SettingsPage dark={dark} onToggleTheme={toggle} />}
        </div>
      </div>
    </div>
  );
}

function LangToggle({ locale, setLocale }: { locale: Locale; setLocale: (l: Locale) => void }) {
  return (
    <div
      role="group"
      aria-label="Language"
      style={{
        display: "flex",
        border: "1px solid var(--border)",
        borderRadius: 999,
        overflow: "hidden",
        background: "var(--card)",
      }}
    >
      {(["vi", "en"] as const).map((l) => {
        const on = locale === l;
        return (
          <button
            key={l}
            type="button"
            onClick={() => setLocale(l)}
            aria-pressed={on}
            style={{
              border: "none",
              padding: "4px 10px",
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: ".4px",
              background: on ? "var(--accent-soft)" : "transparent",
              color: on ? "var(--accent)" : "var(--text-3)",
            }}
          >
            {l.toUpperCase()}
          </button>
        );
      })}
    </div>
  );
}

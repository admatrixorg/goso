import { useCallback, useEffect, useRef, useState } from "react";
import { AgentsPage } from "./pages/AgentsPage";
import { SessionsPage, type SessionsPageHandle } from "./pages/SessionsPage";
import { ChatPage } from "./pages/ChatPage";
import { ConnectorsPage } from "./pages/Connectors";
import { FunctionsPage } from "./pages/FunctionsPage";
import { EventsPage } from "./pages/Events";
import { OverviewPage } from "./pages/OverviewPage";
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
import { PendingPage } from "./pages/PendingPage";
import { Icon, type IconName } from "./ui/Icon";
import { Avatar } from "./ui/Avatar";
import { CommandPalette } from "./ui/CommandPalette";
import { GatewayStatus } from "./ui/GatewayStatus";
import { isDemoMode } from "./demo/mode";
import { demoOverviewItems, demoTop, demoWorkExtra } from "./demo/nav";
import { useI18n, type Locale } from "./i18n";
import { clearSelectedSession, readSelectedSession, writeSelectedSession } from "./api/sessions";

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
  | "pending"
  | "settings";

function liveTop(t: (k: "nav.overview" | "nav.chat" | "nav.connectors" | "nav.events") => string): { id: Tab; label: string }[] {
  return [
    { id: "crm", label: t("nav.overview") },
    { id: "chat", label: t("nav.chat") },
    { id: "connectors", label: t("nav.connectors") },
    { id: "events", label: t("nav.events") },
  ];
}

function visibleTabItems(
  side: { items: { id: Tab; label: string; ic: IconName }[] }[],
  settingsLabel: string,
): { id: Tab; label: string; ic: IconName }[] {
  const seen = new Set<Tab>();
  const out: { id: Tab; label: string; ic: IconName }[] = [];
  for (const g of side) {
    for (const i of g.items) {
      if (seen.has(i.id)) continue;
      seen.add(i.id);
      out.push(i);
    }
  }
  if (!seen.has("settings")) out.push({ id: "settings", label: settingsLabel, ic: "gear" });
  return out;
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
        { id: "pending", label: t("nav.pending"), ic: "hourglass" },
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
  const [sessionId, setSessionId] = useState(() => readSelectedSession()?.id ?? "");
  const [sessionLabel, setSessionLabel] = useState(() => readSelectedSession()?.label ?? "");
  const sessionsRef = useRef<SessionsPageHandle>(null);
  const [tab, setTab] = useState<Tab>(() => {
    if (typeof window !== "undefined" && window.location.hash.startsWith("#traces")) return "traces";
    return DEMO ? "home" : "crm";
  });

  function pickSession(id: string, label?: string) {
    const named = label?.trim() || id;
    setSessionId(id);
    setSessionLabel(named);
    writeSelectedSession({ id, label: named });
  }

  function dropSession(id: string) {
    if (sessionId !== id) return;
    setSessionId("");
    setSessionLabel("");
    clearSelectedSession();
  }
  const { dark, toggle } = useTheme();
  const [q, setQ] = useState("");
  const [paletteOpen, setPaletteOpen] = useState(false);
  const skipHeaderFocus = useRef(false);
  const headerSearchRef = useRef<HTMLInputElement>(null);

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
            { id: "pending" as const, label: t("nav.pending"), ic: "hourglass" as const },
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

  useEffect(() => {
    const onHash = () => {
      if (window.location.hash.startsWith("#traces")) setTab("traces");
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const openPalette = useCallback(() => setPaletteOpen(true), []);
  const closePalette = useCallback(() => {
    setPaletteOpen(false);
    setQ("");
    skipHeaderFocus.current = true;
    headerSearchRef.current?.blur();
    window.setTimeout(() => {
      skipHeaderFocus.current = false;
    }, 0);
  }, []);
  const paletteItems = visibleTabItems(side, t("nav.settings"));

  return (
    <div className="z-shell">
      <div className="z-topbar">
        <div className="z-brand">
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
          <div className="z-wide-only" style={{ fontWeight: 600, fontSize: 14, letterSpacing: "-.2px", color: "var(--text)" }}>ZAgent</div>
        </div>
        <div className="z-top-tabs">
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
        </div>
        <div className="z-topbar-spacer" />
        <div className="z-chrome-actions">
          <GatewayStatus />
          <div
            data-ig="search"
            className="z-header-search"
            style={{
              background: "var(--surface-2)",
              borderRadius: 8,
              padding: "6px 12px",
              color: "var(--text-4)",
              fontSize: 12.5,
              display: "flex",
              gap: 8,
              alignItems: "center",
            }}
          >
            <Icon name="search" size={13} />
            <input
              ref={headerSearchRef}
              className="z-field"
              value={q}
              onChange={(e) => {
                setQ(e.target.value);
                setPaletteOpen(true);
              }}
              onFocus={() => {
                if (skipHeaderFocus.current) return;
                openPalette();
              }}
              placeholder={t("chrome.search")}
              aria-label={t("chrome.search")}
              aria-expanded={paletteOpen}
              aria-haspopup="dialog"
              autoComplete="off"
              spellCheck={false}
              style={{ border: "none", background: "transparent", padding: 0, minHeight: 0, width: "100%" }}
            />
          </div>
          <LangToggle locale={locale} setLocale={setLocale} />
          <button
            type="button"
            onClick={toggle}
            aria-label={dark ? t("chrome.dark") : t("chrome.light")}
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
            ◐ <span className="z-wide-only">{dark ? t("chrome.dark") : t("chrome.light")}</span>
          </button>
          <span data-ig="bell" style={{ position: "relative", color: "var(--text-2)", display: "flex", alignItems: "center" }}>
            <span data-ig-part="">
              <Icon name="bell" size={15} />
            </span>
          </span>
          <Avatar initials="G" size={27} />
        </div>
      </div>

      <div className="z-body">
        <nav className="z-sidebar" aria-label={t("chrome.nav")}>
          <button
            type="button"
            data-ig="search"
            className="z-nav-search"
            onClick={openPalette}
            aria-haspopup="dialog"
            aria-expanded={paletteOpen}
            aria-label={t("chrome.quickSearch")}
            title={t("chrome.quickSearch")}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              width: "100%",
              background: "var(--card)",
              border: "1px solid var(--border)",
              borderRadius: 9,
              padding: "7px 10px",
              fontSize: 12.5,
              color: "var(--text-4)",
              minHeight: 34,
              textAlign: "left",
            }}
          >
            <span className="z-narrow-only">
              <Icon name="search" size={15} />
            </span>
            <span className="z-wide-only" style={{ flex: 1 }}>{t("chrome.quickSearch")}</span>
            <span className="z-wide-only" style={{ fontSize: 11, color: "var(--text-4)", fontWeight: 500 }}>⌘K</span>
          </button>
          {side.map((g) => {
            const items = g.items;
            if (!items.length) return null;
            return (
            <div key={g.group} style={{ marginTop: 14 }}>
              <div
                className="z-nav-group"
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
                    className="z-nav-item"
                    onClick={() => go(i.id)}
                    aria-current={on ? "page" : undefined}
                    aria-label={i.label}
                    title={i.label}
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
                    <span className="z-wide-only" style={{ flex: 1 }}>{i.label}</span>
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
              className="z-nav-item"
              onClick={() => go("settings")}
              aria-current={tab === "settings" ? "page" : undefined}
              aria-label={t("nav.settings")}
              title={t("nav.settings")}
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
              <span className="z-wide-only" style={{ flex: 1 }}>{t("nav.settings")}</span>
            </button>
            <div
              className="z-nav-foot"
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
              <span className="z-wide-only" style={{ flex: 1 }}>GOSO</span>
            </div>
          </div>
        </nav>

        <div className="z-main">
          {DEMO && tab === "home" && <HomePage onMeetings={() => go("meetings")} onChat={() => go("chat")} />}
          {DEMO && tab === "meetings" && <MeetingsPage />}
          {DEMO && tab === "tasks" && <TasksPage onChat={() => go("chat")} />}
          {tab === "crm" && <OverviewPage />}
          {tab === "heatmap" && <HeatmapPage />}
          {tab === "agents" && <AgentsPage />}
          {tab === "sessions" && (
            <SessionsPage
              selectedId={sessionId}
              onPick={(id, label) => {
                pickSession(id, label);
                setTab("chat");
              }}
              onDeleted={dropSession}
            />
          )}
          {tab === "chat" && (
            <div className="z-chat-split">
              <div className="z-chat-sessions">
                <SessionsPage
                  ref={sessionsRef}
                  compact
                  selectedId={sessionId}
                  onPick={pickSession}
                  onDeleted={dropSession}
                />
              </div>
              <div className="z-chat-transcript">
                <ChatPage
                  sessionId={sessionId}
                  sessionLabel={sessionLabel}
                  onNew={() => sessionsRef.current?.focusCreate()}
                  onGone={dropSession}
                />
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
          {tab === "pending" && <PendingPage />}
          {tab === "connectors" && <ConnectorsPage />}
          {tab === "functions" && <FunctionsPage />}
          {tab === "events" && <EventsPage />}
          {tab === "settings" && <SettingsPage dark={dark} onToggleTheme={toggle} />}
        </div>
      </div>
      <CommandPalette
        open={paletteOpen}
        query={q}
        items={paletteItems}
        title={t("palette.title")}
        empty={t("palette.empty")}
        hint={t("palette.hint")}
        placeholder={t("chrome.search")}
        onQuery={setQ}
        onOpen={openPalette}
        onClose={closePalette}
        onPick={go}
      />
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

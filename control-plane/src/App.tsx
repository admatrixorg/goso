import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AgentsPage } from "./pages/AgentsPage";
import { SessionsPage, type SessionsPageHandle } from "./pages/SessionsPage";
import { ChatPage } from "./pages/ChatPage";
import { ConnectorsPage } from "./pages/Connectors";
import { FunctionsPage } from "./pages/FunctionsPage";
import { EventsPage } from "./pages/Events";
import { ActivityPage } from "./pages/ActivityPage";
import { LogsPage } from "./pages/LogsPage";
import { TenantsPage } from "./pages/TenantsPage";
import { ApiKeysPage } from "./pages/ApiKeysPage";
import { PackagesPage } from "./pages/PackagesPage";
import { ApprovalsPage } from "./pages/ApprovalsPage";
import { ImportExportPage } from "./pages/ImportExportPage";
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
import { TracesPage } from "./pages/TracesPage";
import { PendingPage } from "./pages/PendingPage";
import { ContactsPage } from "./pages/ContactsPage";
import { NodesPage } from "./pages/NodesPage";
import { WorkstationsPage } from "./pages/WorkstationsPage";
import { TTSPage } from "./pages/TTSPage";
import { KnowledgeGraphPage } from "./pages/KnowledgeGraphPage";
import { StoragePage } from "./pages/StoragePage";
import { BackupPage, DecisionFlowPage, DecisionPage, HooksPage, MeetingsLivePage } from "./pages/FeaturePages";
import { hashForTab, parseHash, type Tab } from "./api/hash-route";
import { Icon, type IconName } from "./ui/Icon";
import { navIcon } from "./ui/nav-icons";
import { CommandPalette } from "./ui/CommandPalette";
import { GatewayStatus } from "./ui/GatewayStatus";
import { isDemoMode } from "./demo/mode";
import { demoOverviewItems, demoWorkExtra } from "./demo/nav";
import { useI18n, type Locale } from "./i18n";
import { clearSelectedSession, readSelectedSession, writeSelectedSession } from "./api/sessions";
import { TENANT_STORAGE_KEY } from "./api/client";
import { tenantsApi, type Tenant } from "./api/tenants";

const DEMO = isDemoMode();

export type { Tab };

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
  const row = (id: Tab, label: string) => ({ id, label, ic: navIcon(id) });
  return [
    {
      group: t("nav.group.core"),
      items: [row("crm", t("nav.overview")), row("chat", t("nav.chat")), row("agents", t("nav.agents"))],
    },
    {
      group: t("nav.group.conversations"),
      items: [row("sessions", t("nav.sessions")), row("pending", t("nav.pending")), row("contacts", t("nav.contacts"))],
    },
    {
      group: t("nav.group.connectivity"),
      items: [
        row("channels", t("nav.channels")),
        row("connectors", t("nav.connectors")),
        row("nodes", t("nav.nodes")),
        row("workstations", t("nav.workstations")),
      ],
    },
    {
      group: t("nav.group.capabilities"),
      items: [
        row("skills", t("nav.skills")),
        row("decision", t("nav.decision")),
        row("decisionFlow", t("nav.decisionFlow")),
        row("tools", t("nav.tools")),
        row("mcp", t("nav.mcp")),
        row("tts", t("nav.tts")),
        row("cron", t("nav.cron")),
        row("hooks", t("nav.hooks")),
      ],
    },
    {
      group: t("nav.group.data"),
      items: [
        row("memory", t("nav.memory")),
        row("vault", t("nav.vault")),
        row("meetings", t("nav.meetings")),
        row("kg", t("nav.kg")),
        row("storage", t("nav.storage")),
      ],
    },
    {
      group: t("nav.group.monitoring"),
      items: [
        row("traces", t("nav.traces")),
        row("events", t("nav.events")),
        row("activity", t("nav.activity")),
        row("logs", t("nav.logs")),
      ],
    },
    {
      group: t("nav.group.system"),
      items: [
        row("tenants", t("nav.tenants")),
        row("providers", t("nav.providers")),
        row("apikeys", t("nav.apikeys")),
        row("packages", t("nav.packages")),
        row("settings", t("nav.config")),
        row("approvals", t("nav.approvals")),
        row("impexp", t("nav.impexp")),
        row("backup", t("nav.backup")),
      ],
    },
  ];
}

function localZoneLabel(): string {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const city = zone.split("/").pop()?.replace(/_/g, " ");
    return city ? `Auto (${city})` : "Auto (Local)";
  } catch {
    return "Auto (Local)";
  }
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
    if (typeof window === "undefined") return "crm";
    return parseHash(window.location.hash, { demo: DEMO }).tab;
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
  const [accountOpen, setAccountOpen] = useState(false);
  const zoneLabel = useMemo(() => localZoneLabel(), []);

  const liveGroups = liveSide(t);
  const side = DEMO
    ? liveGroups.map((group, index) => {
        if (index === 0) {
          return {
            ...group,
            items: [
              ...demoOverviewItems(locale),
              ...group.items.filter((item) => item.id !== "crm"),
            ],
          };
        }
        if (index === 1) return { ...group, items: [...group.items, ...demoWorkExtra(locale)] };
        return group;
      })
    : liveGroups;

  function applyHash(hash: string, mode: "push" | "replace" | "sync") {
    const parsed = parseHash(hash, { demo: DEMO });
    if (parsed.rewrite || (mode === "replace" && parsed.canonical && window.location.hash !== parsed.canonical)) {
      const next = parsed.canonical || "#/overview";
      window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}${next}`);
    } else if (mode === "push") {
      const next = parsed.canonical || hashForTab(parsed.tab, parsed, { demo: DEMO });
      if (window.location.hash !== next) window.location.hash = next;
    }
    setTab(parsed.tab);
  }

  function go(id: Tab) {
    const current = typeof window === "undefined" ? parseHash("", { demo: DEMO }) : parseHash(window.location.hash, { demo: DEMO });
    const extra =
      id === "settings" && current.tab === "settings"
        ? { settingsPage: current.settingsPage }
        : id === "traces" && current.tab === "traces"
          ? { traceId: current.traceId }
          : undefined;
    const next = hashForTab(id, extra, { demo: DEMO });
    if (typeof window === "undefined") {
      setTab(id);
      return;
    }
    applyHash(next, "push");
    setTab(id);
  }

  useEffect(() => {
    applyHash(window.location.hash, "replace");
    const onHash = () => applyHash(window.location.hash, "sync");
    window.addEventListener("hashchange", onHash);
    window.addEventListener("popstate", onHash);
    return () => {
      window.removeEventListener("hashchange", onHash);
      window.removeEventListener("popstate", onHash);
    };
  }, []);

  const openPalette = useCallback(() => setPaletteOpen(true), []);
  const closePalette = useCallback(() => {
    setPaletteOpen(false);
    setQ("");
  }, []);
  const paletteItems = visibleTabItems(side, t("nav.settings"));
  const pageLabel = paletteItems.find((item) => item.id === tab)?.label;

  useEffect(() => {
    document.title = pageLabel ? `${pageLabel} — Goso` : "Goso";
  }, [pageLabel]);

  return (
    <div className="z-shell">
      <header className="z-topbar">
        <div className="z-topbar-spacer" />
        <div className="z-chrome-actions">
          <LangToggle locale={locale} setLocale={setLocale} />
          <span className="z-gbtn" title={t("chrome.zone")}>
            <Icon name="clock" size={16} />
            <span className="z-wide-only">{zoneLabel}</span>
          </span>
          <button type="button" className="z-gbtn" aria-label={t("chrome.system")} title={t("chrome.system")} onClick={() => go("settings")}>
            <Icon name="settings" size={16} />
          </button>
          <button type="button" className="z-gbtn" onClick={toggle} aria-label={dark ? t("chrome.dark") : t("chrome.light")}>
            {dark ? <Icon name="sun" size={16} /> : <Icon name="moon" size={16} />}
          </button>
          <AccountMenu
            open={accountOpen}
            label={t("chrome.account")}
            orgLabel={t("chrome.account.org")}
            keysLabel={t("chrome.account.keys")}
            logoutLabel={t("chrome.account.logout")}
            onToggle={() => setAccountOpen((v) => !v)}
            onClose={() => setAccountOpen(false)}
            onOrg={() => {
              setAccountOpen(false);
              go("tenants");
            }}
            onKeys={() => {
              setAccountOpen(false);
              go("apikeys");
            }}
          />
        </div>
      </header>

      <div className="z-body">
        <nav className="z-sidebar" aria-label={t("chrome.nav")}>
          <div className="z-nav-scroll">
          {side.map((g) => {
            const items = g.items;
            if (!items.length) return null;
            return (
            <div key={g.group} style={{ marginTop: 14 }}>
              <div
                className="z-nav-group"
                style={{
                  fontSize: 10.5,
                  fontWeight: 600,
                  letterSpacing: ".06em",
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
                    <Icon name={i.ic} size={16} />
                    <span className="z-wide-only" style={{ flex: 1 }}>{i.label}</span>
                  </button>
                );
              })}
            </div>
            );
          })}
          </div>
          <div className="z-org">
            <div className="z-org-row">
              <span className="z-wide-only">GOSO</span>
            </div>
            <GatewayStatus quiet />
          </div>
        </nav>

        <div className="z-main">
          {DEMO && tab === "home" && <HomePage onMeetings={() => go("meetings")} onChat={() => go("chat")} />}
          {tab === "meetings" && (DEMO ? <MeetingsPage /> : <MeetingsLivePage />)}
          {DEMO && tab === "tasks" && <TasksPage onChat={() => go("chat")} />}
          {tab === "crm" && <OverviewPage />}
          {tab === "decision" && <DecisionPage />}
          {tab === "decisionFlow" && <DecisionFlowPage />}
          {tab === "hooks" && <HooksPage />}
          {tab === "backup" && <BackupPage />}
          {tab === "heatmap" && <HeatmapPage />}
          {tab === "agents" && <AgentsPage />}
          {tab === "sessions" && (
            <SessionsPage
              selectedId={sessionId}
              onPick={(id, label) => {
                pickSession(id, label);
                go("chat");
              }}
                  onDeleted={dropSession}
                  onRenamed={(id, label) => {
                    if (sessionId === id) pickSession(id, label);
                  }}
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
                  onRenamed={(id, label) => {
                    if (sessionId === id) pickSession(id, label);
                  }}
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
          {tab === "kg" && <KnowledgeGraphPage />}
          {tab === "storage" && <StoragePage />}
          {tab === "providers" && <ProvidersPage />}
          {tab === "channels" && <ChannelsPage />}
          {tab === "nodes" && <NodesPage />}
          {tab === "workstations" && <WorkstationsPage />}
          {tab === "traces" && <TracesPage />}
          {tab === "pending" && <PendingPage />}
          {tab === "contacts" && <ContactsPage />}
          {tab === "connectors" && <ConnectorsPage />}
          {(tab === "functions" || tab === "skills" || tab === "tools" || tab === "mcp" || tab === "cron") && (
            <FunctionsPage focus={tab === "functions" ? "tools" : tab} />
          )}
          {tab === "tts" && <TTSPage />}
          {tab === "events" && <EventsPage />}
          {tab === "activity" && <ActivityPage />}
          {tab === "logs" && <LogsPage />}
          {tab === "tenants" && <TenantsPage />}
          {tab === "apikeys" && <ApiKeysPage />}
          {tab === "packages" && <PackagesPage />}
          {tab === "approvals" && <ApprovalsPage />}
          {tab === "impexp" && <ImportExportPage />}
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
        suppressShortcut={tab === "chat"}
      />
    </div>
  );
}

function AccountMenu({
  open,
  label,
  orgLabel,
  keysLabel,
  logoutLabel,
  onToggle,
  onClose,
  onOrg,
  onKeys,
}: {
  open: boolean;
  label: string;
  orgLabel: string;
  keysLabel: string;
  logoutLabel: string;
  onToggle: () => void;
  onClose: () => void;
  onOrg: () => void;
  onKeys: () => void;
}) {
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [current, setCurrent] = useState("");
  useEffect(() => {
    if (!open) return;
    let stop = false;
    void tenantsApi
      .list()
      .then((j) => {
        if (stop) return;
        setTenants(j.tenants ?? []);
        setCurrent(j.current?.name || j.current?.id || "");
      })
      .catch(() => {
        if (!stop) setTenants([]);
      });
    const close = (e: PointerEvent) => {
      const node = e.target as HTMLElement | null;
      if (node?.closest?.(".z-account")) return;
      onClose();
    };
    window.addEventListener("pointerdown", close);
    return () => {
      stop = true;
      window.removeEventListener("pointerdown", close);
    };
  }, [open, onClose]);

  function pickTenant(id: string) {
    try {
      localStorage.setItem(TENANT_STORAGE_KEY, id);
    } catch {
      /* private mode */
    }
    window.location.reload();
  }

  function logout() {
    try {
      localStorage.removeItem("goso_token");
    } catch {
      /* private mode */
    }
    onClose();
    window.location.reload();
  }

  return (
    <div className="z-account">
      <button type="button" className="z-gbtn" aria-label={label} aria-expanded={open} onClick={onToggle}>
        <Icon name="user" size={16} />
        <span className="z-wide-only">{label}</span>
      </button>
      {open ? (
        <div className="z-account-menu" role="menu">
          {current ? <div className="z-account-current">{current}</div> : null}
          {tenants.map((row) => (
            <button key={row.id} type="button" role="menuitem" onClick={() => pickTenant(row.id)}>
              {row.name || row.id}
            </button>
          ))}
          {tenants.length ? <hr /> : null}
          <button type="button" role="menuitem" onClick={onOrg}>
            {orgLabel}
          </button>
          <button type="button" role="menuitem" onClick={onKeys}>
            {keysLabel}
          </button>
          <hr />
          <button type="button" role="menuitem" onClick={logout}>
            {logoutLabel}
          </button>
        </div>
      ) : null}
    </div>
  );
}

function LangToggle({ locale, setLocale }: { locale: Locale; setLocale: (l: Locale) => void }) {
  const { t } = useI18n();
  const next = locale === "vi" ? "en" : "vi";
  return (
    <button type="button" className="z-gbtn" aria-label={t("chrome.lang")} title={t("chrome.lang")} onClick={() => setLocale(next)}>
      <Icon name="languages" size={16} />
      <span className="z-wide-only">{locale === "vi" ? "Tiếng Việt" : "English"}</span>
    </button>
  );
}

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
import { ConfirmDialog } from "./ui/ConfirmDialog";
import { GatewayStatus } from "./ui/GatewayStatus";
import { cn } from "./lib/cn";
import { isDemoMode } from "./demo/mode";
import { demoOverviewItems, demoWorkExtra } from "./demo/nav";
import { useI18n, type Locale } from "./i18n";
import { clearSelectedSession, readSelectedSession, writeSelectedSession } from "./api/sessions";
import { TENANT_STORAGE_KEY } from "./api/client";
import { tenantsApi, type Tenant } from "./api/tenants";
import { LoginPage, hasBrowserToken, tokenIsEnvOwned } from "./pages/LoginPage";

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
      items: [row("crm", t("nav.overview")), row("chat", t("nav.chat")), row("agents", t("nav.agents")), row("teams", t("nav.teams"))],
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

const COLLAPSE_KEY = "goso_sidebar_collapsed";

function useTheme() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark") || document.body.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    document.body.classList.toggle("dark", dark);
  }, [dark]);
  return { dark, toggle: () => setDark((d) => !d) };
}

function useIsMobile() {
  const [mobile, setMobile] = useState(() => (typeof window === "undefined" ? false : window.matchMedia("(max-width: 899px)").matches));
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 899px)");
    const on = () => setMobile(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return mobile;
}

export default function App() {
  const { t, locale, setLocale } = useI18n();
  // Gate the shell behind a working admin token. The build can own the token
  // (VITE_GOSO_ADMIN_TOKEN), in which case there is nothing to ask for.
  const [authed, setAuthed] = useState(() => tokenIsEnvOwned() || hasBrowserToken());
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
  const isMobile = useIsMobile();
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(COLLAPSE_KEY) === "1";
    } catch {
      return false;
    }
  });
  const [mobileNav, setMobileNav] = useState(false);
  const [q, setQ] = useState("");
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const zoneLabel = useMemo(() => localZoneLabel(), []);

  function toggleCollapsed() {
    setCollapsed((c) => {
      const next = !c;
      try {
        localStorage.setItem(COLLAPSE_KEY, next ? "1" : "0");
      } catch {
        /* private mode */
      }
      return next;
    });
  }

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
    if (isMobile) setMobileNav(false);
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

  if (!authed) {
    return <LoginPage onDone={() => setAuthed(true)} />;
  }

  const showLabels = !collapsed || isMobile;

  return (
    <div className="flex h-dvh overflow-hidden">
      {isMobile && mobileNav ? (
        <div className="fixed inset-0 z-40 bg-black/50 transition-opacity" onClick={() => setMobileNav(false)} />
      ) : null}
      <aside
        aria-label={t("chrome.nav")}
        className={cn(
          "flex h-full flex-col overflow-hidden overscroll-none border-r bg-sidebar text-sidebar-foreground transition-all duration-200",
          isMobile
            ? cn("fixed inset-y-0 left-0 z-50 w-64", mobileNav ? "translate-x-0" : "-translate-x-full")
            : collapsed
              ? "w-16"
              : "w-64",
        )}
      >
        <div className="flex h-14 items-center border-b px-4">
          {showLabels ? (
            <div className="flex items-center gap-2.5">
              <span className="h-8 w-8 shrink-0" aria-hidden="true">
                <img src="/admatrix-mark-transparent-v3.png" alt="" className="h-full w-full object-contain" />
              </span>
              <span className="bg-gradient-to-r from-blue-700 via-blue-500 to-cyan-400 bg-clip-text text-lg font-bold tracking-tight text-transparent">
                GOSO
              </span>
            </div>
          ) : (
            <span className="mx-auto h-7 w-7" aria-label="GOSO">
              <img src="/admatrix-mark-transparent-v3.png" alt="" className="h-full w-full object-contain" />
            </span>
          )}
        </div>
        <nav className="flex-1 space-y-4 overflow-y-auto overscroll-contain px-2 py-4">
        {side.map((g) => {
          const items = g.items;
          if (!items.length) return null;
          return (
          <div key={g.group} className="space-y-1">
            {showLabels ? (
              <p className="px-3 py-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{g.group}</p>
            ) : (
              <div className="mx-auto my-1 h-px w-6 bg-border" />
            )}
            <div className={cn(collapsed && !isMobile && "flex flex-col items-center")}>
            {items.map((i) => {
              const on = tab === i.id;
              return (
                <button
                  key={i.id}
                  type="button"
                  onClick={() => go(i.id)}
                  aria-current={on ? "page" : undefined}
                  aria-label={i.label}
                  title={i.label}
                  className={cn(
                    "flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground",
                    on && "bg-accent text-accent-foreground font-medium",
                    collapsed && !isMobile && "justify-center px-2",
                  )}
                >
                  <Icon name={i.ic} size={16} className="h-4 w-4 shrink-0" />
                  {showLabels ? <span className="truncate">{i.label}</span> : null}
                </button>
              );
            })}
            </div>
          </div>
          );
        })}
        </nav>
        <div className={cn("shrink-0 border-t py-3", collapsed && !isMobile ? "flex justify-center px-2" : "px-4")}>
          <GatewayStatus quiet />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="landscape-compact flex h-14 items-center justify-between border-b bg-background px-4">
          <button
            type="button"
            className="cursor-pointer rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            aria-label={isMobile ? t("chrome.menu") : collapsed ? t("chrome.expand") : t("chrome.collapse")}
            title={isMobile ? t("chrome.menu") : collapsed ? t("chrome.expand") : t("chrome.collapse")}
            onClick={() => (isMobile ? setMobileNav(true) : toggleCollapsed())}
          >
            <Icon name={isMobile ? "menu" : collapsed ? "panel-left" : "panel-left-close"} size={16} className="h-4 w-4" />
          </button>
          <div className="flex items-center gap-2">
            <LangToggle locale={locale} setLocale={setLocale} />
            <span className="hidden items-center gap-1 rounded-md px-2 py-1.5 text-sm text-muted-foreground sm:inline-flex" title={t("chrome.zone")}>
              <Icon name="clock" size={16} className="h-4 w-4" />
              {zoneLabel}
            </span>
            <button type="button" className="cursor-pointer rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-accent-foreground" aria-label={t("chrome.system")} title={t("chrome.system")} onClick={() => go("settings")}>
              <Icon name="settings" size={16} className="h-4 w-4" />
            </button>
            <button type="button" className="cursor-pointer rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-accent-foreground" onClick={toggle} aria-label={dark ? t("chrome.dark") : t("chrome.light")}>
              {dark ? <Icon name="sun" size={16} className="h-4 w-4" /> : <Icon name="moon" size={16} className="h-4 w-4" />}
            </button>
            <AccountMenu
              open={accountOpen}
              label={t("chrome.account")}
              orgLabel={t("chrome.account.org")}
              keysLabel={t("chrome.account.keys")}
              logoutLabel={t("chrome.account.logout")}
              logoutConfirm={t("chrome.logoutConfirm")}
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
              onLogout={() => {
                setAccountOpen(false);
                setAuthed(false);
              }}
            />
          </div>
        </header>

        <main className="z-main min-w-0 flex-1 overflow-y-auto">
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
        </main>
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
  logoutConfirm,
  onToggle,
  onClose,
  onOrg,
  onKeys,
  onLogout,
}: {
  open: boolean;
  label: string;
  orgLabel: string;
  keysLabel: string;
  logoutLabel: string;
  logoutConfirm: string;
  onToggle: () => void;
  onClose: () => void;
  onOrg: () => void;
  onKeys: () => void;
  onLogout: () => void;
}) {
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [current, setCurrent] = useState("");
  const [logoutOpen, setLogoutOpen] = useState(false);
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
      if (node?.closest?.(".goso-account")) return;
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
    // Drop straight back to the gate. A reload would race the gate's initial
    // state, and the point of logging out is to be asked again.
    onLogout();
  }

  return (
    <div className="relative goso-account">
      <button type="button" className="flex cursor-pointer items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground" aria-label={label} aria-expanded={open} onClick={onToggle}>
        <Icon name="user" size={16} className="h-4 w-4 shrink-0" />
        <span className="hidden max-w-40 truncate sm:inline">{label}</span>
        <Icon name="chevron-down" size={12} className="h-3 w-3 opacity-50" />
      </button>
      {open ? (
        <div className="absolute right-0 top-[calc(100%+8px)] z-50 w-56 rounded-lg border bg-popover p-1 text-popover-foreground shadow-md" role="menu">
          {current ? <div className="mb-1 truncate border-b px-2 py-1.5 text-sm font-medium">{current}</div> : null}
          {tenants.map((row) => (
            <button key={row.id} type="button" role="menuitem" className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent" onClick={() => pickTenant(row.id)}>
              <Icon name="building" size={14} className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="flex-1 truncate text-left">{row.name || row.id}</span>
            </button>
          ))}
          {tenants.length ? <div className="my-1 border-t" /> : null}
          <button type="button" role="menuitem" className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent" onClick={onOrg}>
            <Icon name="building" size={14} className="h-3.5 w-3.5" />
            {orgLabel}
          </button>
          <button type="button" role="menuitem" className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent" onClick={onKeys}>
            <Icon name="key" size={14} className="h-3.5 w-3.5" />
            {keysLabel}
          </button>
          <div className="my-1 border-t" />
          <button
            type="button"
            role="menuitem"
            className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm text-destructive hover:bg-accent"
            onClick={() => {
              onClose();
              setLogoutOpen(true);
            }}
          >
            <Icon name="logout" size={14} className="h-3.5 w-3.5" />
            {logoutLabel}
          </button>
        </div>
      ) : null}
      <ConfirmDialog
        open={logoutOpen}
        onOpenChange={setLogoutOpen}
        title={logoutLabel}
        description={logoutConfirm}
        confirmLabel={logoutLabel}
        variant="destructive"
        onConfirm={() => {
          setLogoutOpen(false);
          logout();
        }}
      />
    </div>
  );
}

function LangToggle({ locale, setLocale }: { locale: Locale; setLocale: (l: Locale) => void }) {
  const { t } = useI18n();
  const next = locale === "vi" ? "en" : "vi";
  return (
    <button type="button" className="flex cursor-pointer items-center gap-1 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground" aria-label={t("chrome.lang")} title={t("chrome.lang")} onClick={() => setLocale(next)}>
      <Icon name="languages" size={16} className="h-4 w-4" />
      <span className="hidden sm:inline">{locale === "vi" ? "Tiếng Việt" : "English"}</span>
    </button>
  );
}

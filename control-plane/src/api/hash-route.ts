/** Tiny hash router for Control Plane left-nav. No react-router. */

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
  | "skills"
  | "tools"
  | "mcp"
  | "cron"
  | "events"
  | "activity"
  | "logs"
  | "tenants"
  | "apikeys"
  | "packages"
  | "approvals"
  | "impexp"
  | "teams"
  | "vault"
  | "memory"
  | "kg"
  | "storage"
  | "providers"
  | "channels"
  | "webhooks"
  | "traces"
  | "pending"
  | "contacts"
  | "nodes"
  | "workstations"
  | "tts"
  | "settings";

export const SETTINGS_PAGES = [
  "account",
  "users",
  "roles",
  "nicks",
  "quotas",
  "templates",
  "billing",
  "gateway",
  "backup",
  "pairing",
  "theme",
] as const;

export type SettingsPageId = (typeof SETTINGS_PAGES)[number];

/** Bare `#/config` opens Gateway, not CRM Account. */
export const DEFAULT_SETTINGS_PAGE: SettingsPageId = "gateway";

export const DEMO_HASH_TABS = ["home", "tasks", "meetings", "friends", "calendar", "gallery"] as const;
export type DemoHashTab = (typeof DEMO_HASH_TABS)[number];

const TRACE_ID = /^[A-Za-z0-9_-]+$/;
const SETTINGS_SET = new Set<string>(SETTINGS_PAGES);
const DEMO_SET = new Set<string>(DEMO_HASH_TABS);

/** Live tab → path segment. `crm` bookmarks as overview; Config as config. */
const TAB_SEG: Partial<Record<Tab, string>> = {
  crm: "overview",
  heatmap: "heatmap",
  chat: "chat",
  agents: "agents",
  teams: "teams",
  sessions: "sessions",
  pending: "pending",
  contacts: "contacts",
  marketing: "marketing",
  channels: "channels",
  nodes: "nodes",
  workstations: "workstations",
  skills: "skills",
  tools: "tools",
  mcp: "mcp",
  tts: "tts",
  cron: "cron",
  webhooks: "webhooks",
  connectors: "connectors",
  memory: "memory",
  vault: "vault",
  kg: "kg",
  storage: "storage",
  traces: "traces",
  events: "events",
  activity: "activity",
  logs: "logs",
  tenants: "tenants",
  providers: "providers",
  apikeys: "apikeys",
  packages: "packages",
  settings: "config",
  approvals: "approvals",
  impexp: "impexp",
  home: "home",
  tasks: "tasks",
  meetings: "meetings",
  friends: "friends",
  calendar: "calendar",
  gallery: "gallery",
};

const SEG_TAB: Record<string, Tab> = {
  overview: "crm",
  heatmap: "heatmap",
  chat: "chat",
  agents: "agents",
  teams: "teams",
  sessions: "sessions",
  pending: "pending",
  contacts: "contacts",
  marketing: "marketing",
  channels: "channels",
  nodes: "nodes",
  workstations: "workstations",
  skills: "skills",
  tools: "tools",
  functions: "tools",
  mcp: "mcp",
  tts: "tts",
  cron: "cron",
  webhooks: "webhooks",
  connectors: "connectors",
  memory: "memory",
  vault: "vault",
  kg: "kg",
  storage: "storage",
  traces: "traces",
  events: "events",
  activity: "activity",
  logs: "logs",
  tenants: "tenants",
  providers: "providers",
  apikeys: "apikeys",
  packages: "packages",
  config: "settings",
  approvals: "approvals",
  impexp: "impexp",
  home: "home",
  tasks: "tasks",
  meetings: "meetings",
  friends: "friends",
  calendar: "calendar",
  gallery: "gallery",
};

export type HashRoute = {
  tab: Tab;
  settingsPage?: SettingsPageId;
  traceId?: string;
};

export type HashParse = HashRoute & {
  canonical: string;
  rewrite: boolean;
  unknown: boolean;
};

export type HashOpts = { demo?: boolean };

function demoOn(opts?: HashOpts): boolean {
  return opts?.demo === true;
}

function bodyOf(hash: string): string {
  let s = (hash || "").trim();
  if (s.startsWith("#")) s = s.slice(1);
  return s;
}

function segsOf(body: string): string[] {
  return body.replace(/^\/+/, "").replace(/\/+$/, "").split("/").filter(Boolean);
}

function isDemoTab(tab: string): tab is DemoHashTab {
  return DEMO_SET.has(tab);
}

function settingsPageOf(raw?: string): SettingsPageId | undefined {
  if (!raw) return undefined;
  return SETTINGS_SET.has(raw) ? (raw as SettingsPageId) : undefined;
}

function traceIdOf(raw?: string): string | undefined {
  const v = (raw || "").trim();
  if (!v || !TRACE_ID.test(v)) return undefined;
  return v;
}

export function serializeHash(route: HashRoute, opts?: HashOpts): string {
  const demo = demoOn(opts);
  const tab = route.tab === "functions" ? "tools" : route.tab;
  if (isDemoTab(tab) && !demo) return "#/overview";
  if (tab === "crm") return "#/overview";
  if (tab === "settings") {
    const page = route.settingsPage && route.settingsPage !== DEFAULT_SETTINGS_PAGE ? route.settingsPage : undefined;
    return page ? `#/config/${page}` : "#/config";
  }
  if (tab === "traces") {
    const id = traceIdOf(route.traceId);
    return id ? `#/traces/${id}` : "#/traces";
  }
  const seg = TAB_SEG[tab];
  if (!seg) return "#/overview";
  return `#/${seg}`;
}

function parsed(route: HashRoute, opts: HashOpts | undefined, rewrite: boolean, unknown: boolean): HashParse {
  return { ...route, canonical: serializeHash(route, opts), rewrite, unknown };
}

function overview(opts: HashOpts | undefined, rewrite: boolean, unknown: boolean): HashParse {
  return parsed({ tab: "crm" }, opts, rewrite, unknown);
}

/** Parse location.hash. Unknown live hashes rewrite to Overview `#/overview`. */
export function parseHash(hash: string, opts?: HashOpts): HashParse {
  const raw = (hash || "").trim();
  const body = bodyOf(raw);

  if (body === "traces" || body.startsWith("traces/")) {
    const id = traceIdOf(body.slice("traces".length).replace(/^\//, ""));
    return parsed({ tab: "traces", traceId: id }, opts, true, false);
  }

  const segs = segsOf(body);
  if (segs.length === 0) {
    return { tab: "crm", canonical: raw === "#/" ? "#/" : raw === "#" || raw === "" ? "" : "#/", rewrite: false, unknown: false };
  }

  const head = segs[0].toLowerCase();
  if (head === "overview") {
    const extra = segs.length > 1;
    return extra ? overview(opts, true, true) : parsed({ tab: "crm" }, opts, false, false);
  }

  if (head === "config") {
    const pageRaw = segs[1];
    if (!pageRaw) return parsed({ tab: "settings", settingsPage: DEFAULT_SETTINGS_PAGE }, opts, false, false);
    const page = settingsPageOf(pageRaw.toLowerCase());
    if (!page) return parsed({ tab: "settings", settingsPage: DEFAULT_SETTINGS_PAGE }, opts, true, false);
    if (segs.length > 2) return parsed({ tab: "settings", settingsPage: page }, opts, true, false);
    return parsed({ tab: "settings", settingsPage: page }, opts, page === DEFAULT_SETTINGS_PAGE, false);
  }

  if (head === "traces") {
    const id = traceIdOf(segs[1]);
    const extra = segs.length > 2 || Boolean(segs[1] && !id);
    return parsed({ tab: "traces", traceId: id }, opts, extra, false);
  }

  if (head === "functions") {
    return parsed({ tab: "tools" }, opts, true, false);
  }

  const tab = SEG_TAB[head];
  if (!tab) return overview(opts, true, true);
  if (isDemoTab(tab) && !demoOn(opts)) return overview(opts, true, true);
  if (segs.length > 1) return parsed({ tab }, opts, true, false);
  return parsed({ tab }, opts, false, false);
}

export function hashForTab(tab: Tab, extra?: { settingsPage?: SettingsPageId; traceId?: string }, opts?: HashOpts): string {
  return serializeHash({ tab, settingsPage: extra?.settingsPage, traceId: extra?.traceId }, opts);
}

export function liveTabIds(): Tab[] {
  return [
    "crm",
    "heatmap",
    "chat",
    "agents",
    "teams",
    "sessions",
    "pending",
    "contacts",
    "marketing",
    "channels",
    "nodes",
    "workstations",
    "skills",
    "tools",
    "mcp",
    "tts",
    "cron",
    "webhooks",
    "connectors",
    "memory",
    "vault",
    "kg",
    "storage",
    "traces",
    "events",
    "activity",
    "logs",
    "tenants",
    "providers",
    "apikeys",
    "packages",
    "settings",
    "approvals",
    "impexp",
  ];
}

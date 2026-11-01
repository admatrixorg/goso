import type { Tab } from "../api/hash-route";
import type { IconName } from "./Icon";

/** Sidebar and matching page-header icon, one per route. Values are unique. */
export const NAV_ICONS: Partial<Record<Tab, IconName>> = {
  crm: "layout",
  chat: "messages",
  agents: "bot",
  sessions: "history",
  pending: "inbox",
  contacts: "contact",
  channels: "radio",
  connectors: "plug",
  nodes: "network",
  workstations: "monitor",
  skills: "sparkles",
  decision: "gavel",
  decisionFlow: "branch",
  tools: "wrench",
  mcp: "plug-zap",
  tts: "waveform",
  cron: "calendar-clock",
  hooks: "webhook",
  memory: "brain",
  vault: "vault",
  meetings: "video",
  kg: "share",
  storage: "hard-drive",
  traces: "scan",
  events: "rss",
  activity: "activity",
  logs: "scroll",
  tenants: "building",
  providers: "cpu",
  apikeys: "key",
  packages: "package",
  settings: "settings",
  approvals: "shield-check",
  impexp: "swap",
  backup: "archive",
  home: "layout",
  tasks: "check",
  friends: "friends",
  calendar: "cal",
  gallery: "gallery",
  heatmap: "report",
  teams: "layers",
  marketing: "mega",
};

export function navIcon(tab: Tab): IconName {
  return NAV_ICONS[tab] || "layout";
}

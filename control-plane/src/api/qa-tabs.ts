/** Route list and failure rules for `npm run qa:tabs` (v2 §13 rule 2). */

export type TabShot = { tab: string; hash: string; proto: string };

/** Every live sidebar tab, in v2 §1 order, and the prototype hash that shows the same screen. */
export const SIDEBAR_TABS: TabShot[] = [
  { tab: "crm", hash: "#/overview", proto: "overview" },
  { tab: "chat", hash: "#/chat", proto: "chat" },
  { tab: "agents", hash: "#/agents", proto: "agent-setting" },
  { tab: "sessions", hash: "#/sessions", proto: "sessions" },
  { tab: "pending", hash: "#/pending", proto: "pending" },
  { tab: "contacts", hash: "#/contacts", proto: "contacts" },
  { tab: "channels", hash: "#/channels", proto: "channels" },
  { tab: "connectors", hash: "#/connectors", proto: "connectors" },
  { tab: "nodes", hash: "#/nodes", proto: "nodes" },
  { tab: "workstations", hash: "#/workstations", proto: "workstations" },
  { tab: "skills", hash: "#/skills", proto: "skills" },
  { tab: "decision", hash: "#/decision", proto: "decision" },
  { tab: "decisionFlow", hash: "#/decision-flow", proto: "decision-flow" },
  { tab: "tools", hash: "#/tools", proto: "tools" },
  { tab: "mcp", hash: "#/mcp", proto: "mcp" },
  { tab: "tts", hash: "#/tts", proto: "tts" },
  { tab: "cron", hash: "#/cron", proto: "cron" },
  { tab: "hooks", hash: "#/hooks", proto: "hooks" },
  { tab: "memory", hash: "#/memory", proto: "memory" },
  { tab: "vault", hash: "#/vault", proto: "vault" },
  { tab: "meetings", hash: "#/meetings", proto: "meetings" },
  { tab: "kg", hash: "#/kg", proto: "kgraph" },
  { tab: "storage", hash: "#/storage", proto: "storage" },
  { tab: "traces", hash: "#/traces", proto: "traces" },
  { tab: "events", hash: "#/events", proto: "events" },
  { tab: "activity", hash: "#/activity", proto: "activity" },
  { tab: "logs", hash: "#/logs", proto: "logs" },
  { tab: "tenants", hash: "#/tenants", proto: "tenants" },
  { tab: "providers", hash: "#/providers", proto: "providers" },
  { tab: "apikeys", hash: "#/apikeys", proto: "api-keys" },
  { tab: "packages", hash: "#/packages", proto: "packages" },
  { tab: "settings", hash: "#/config", proto: "config" },
  { tab: "approvals", hash: "#/approvals", proto: "approvals" },
  { tab: "impexp", hash: "#/impexp", proto: "import-export" },
  { tab: "backup", hash: "#/backup", proto: "backup-restore" },
];

/** Hashes that must rewrite onto a canonical tab. */
export const HASH_ALIASES: { from: string; to: string }[] = [
  { from: "#projects", to: "#/chat" },
  { from: "#agent-team", to: "#/agents" },
  { from: "#agent-link", to: "#/agents" },
  { from: "#webhooks", to: "#/hooks" },
  { from: "#/functions", to: "#/tools" },
  { from: "#api-keys", to: "#/apikeys" },
  { from: "#import-export", to: "#/impexp" },
  { from: "#backup-restore", to: "#/backup" },
  { from: "#kgraph", to: "#/kg" },
  { from: "#meetings", to: "#/meetings" },
  { from: "#decision-flow", to: "#/decision-flow" },
];

/** Reachable from a host page, not a sidebar item. */
export const FOLDED_HASHES = ["#/teams", "#/heatmap", "#/marketing"];

/**
 * A screen that says the UI was never built. Honest empty copy
 * (“Chưa có cuộc họp”, “Chưa có dữ liệu”, “Chưa có hook”) is not in this list:
 * those pages are waiting on a later SPEC and must stay visible as empty.
 */
export const UNFINISHED_MARKERS = [
  "chưa dựng",
  "màn hình này không có trên bản non-demo",
  "trạng thái đang phát triển — không có api",
];

export function unfinishedMarker(text: string): string | null {
  const hay = text.toLowerCase();
  for (const marker of UNFINISHED_MARKERS) {
    if (hay.includes(marker)) return marker;
  }
  return null;
}

/**
 * goso-crm listens on :8082 and is proxied as /crm-api. It is a separate product.
 * Overview and the folded heatmap call it when present. A down CRM is not a Goso gateway failure.
 * Favicon misses are chrome, not a page dependency.
 */
export function ignoredFailure(url: string): boolean {
  try {
    const path = new URL(url, "http://127.0.0.1").pathname;
    if (path === "/favicon.ico") return true;
    if (path === "/crm-api" || path.startsWith("/crm-api/")) return true;
  } catch {
    return false;
  }
  return false;
}

/** Browser “failed to load” lines are duplicates of the response listener. */
export function ignoredConsole(text: string): boolean {
  if (/failed to load resource/i.test(text)) return true;
  if (/favicon|\/crm-api/i.test(text)) return true;
  return false;
}

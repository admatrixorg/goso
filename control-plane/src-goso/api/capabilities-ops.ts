import { confirmNamed, type ConfirmFn } from "./confirm.ts";
import {
  CONNECTOR_TRANSPORTS,
  normalizeTransport,
  type ConnectorInfo,
  type ConnectorTransport,
} from "./function-ops.ts";
import { classifyPageState, inventoryBlocksMutation, isPermissionError, type PageLoadKind, type PageState } from "./page-state.ts";
import type { LastSecret } from "./webhooks-ops.ts";

export type ToolViewKind =
  | "loading"
  | "permission"
  | "error"
  | "stale"
  | "no_agent"
  | "no_selection"
  | "unsupported"
  | "empty"
  | "filtered_empty"
  | "ready";

export type CronSpecParse =
  | { ok: true; kind: "interval" | "five" }
  | { ok: false; reason: "empty" | "once" | "invalid" };

export const SKILL_UNAVAILABLE = ["rescan", "install", "deps", "enable", "edit", "bulk", "status"] as const;
export type SkillUnavailable = (typeof SKILL_UNAVAILABLE)[number];

export const MCP_UNAVAILABLE = [
  "display_name",
  "args",
  "env_values",
  "agent_hints",
  "tool_prefix",
  "timeout",
  "user_credentials",
  "credential_clear",
] as const;

const SECRET_ROW_KEYS = new Set(["token", "api_key", "secret", "password", "hmac_key", "authorization"]);

export function isRouteUnsupported(err: unknown): boolean {
  const s = err instanceof Error ? err.message : String(err ?? "");
  return /^404\b/.test(s.trim());
}

export function isHttpEndpoint(raw: string): boolean {
  return /^https?:\/\/\S+$/i.test((raw || "").trim());
}

export function skillNameOk(name: string): boolean {
  return /^[a-z0-9_-]{1,64}$/.test((name || "").trim());
}

export function filterByQuery<T>(rows: T[], query: string, text: (row: T) => string): T[] {
  const q = (query || "").trim().toLowerCase();
  if (!q) return rows;
  return rows.filter((row) => text(row).toLowerCase().includes(q));
}

/** Agent inventory + tools list: missing selection is not true-empty. */
export function classifyToolView(input: {
  agentsLoading: boolean;
  agentsLoaded: boolean;
  agentsError: unknown | null | undefined;
  agentCount: number;
  agentId: string;
  toolsLoading: boolean;
  toolsLoaded: boolean;
  toolsError: unknown | null | undefined;
  toolCount: number;
  visibleCount?: number;
}): ToolViewKind {
  const agents = classifyPageState({
    loading: input.agentsLoading,
    loaded: input.agentsLoaded,
    error: input.agentsError,
    itemCount: input.agentCount,
    keepStale: input.agentsLoaded && input.agentCount > 0,
  });
  if (agents.kind === "loading") return "loading";
  if (agents.kind === "permission") return "permission";
  if (agents.kind === "error") return "error";
  if (agents.kind === "stale") return "stale";
  if (agents.kind === "empty") return "no_agent";
  if (!(input.agentId || "").trim()) return "no_selection";

  if (input.toolsError && isRouteUnsupported(input.toolsError)) return "unsupported";
  const tools = classifyPageState({
    loading: input.toolsLoading,
    loaded: input.toolsLoaded,
    error: input.toolsError,
    itemCount: input.toolCount,
    keepStale: input.toolsLoaded && input.toolCount > 0,
  });
  if (tools.kind === "loading") return "loading";
  if (tools.kind === "permission") return "permission";
  if (tools.kind === "error") return "error";
  if (tools.kind === "stale") return "stale";
  if (tools.kind === "empty") return "empty";
  const visible = input.visibleCount == null ? input.toolCount : input.visibleCount;
  if (tools.showItems && input.toolCount > 0 && visible === 0) return "filtered_empty";
  return "ready";
}

export function toolViewBlocksMutation(kind: ToolViewKind): boolean {
  return kind === "error" || kind === "permission" || kind === "unsupported" || kind === "no_agent" || kind === "no_selection" || kind === "loading";
}

export function parseCronSpec(raw: string): CronSpecParse {
  const s = (raw || "").trim();
  if (!s) return { ok: false, reason: "empty" };
  const lower = s.toLowerCase();
  if (lower === "once" || lower.startsWith("once:") || lower.startsWith("at:") || /\bonce\b/.test(lower)) {
    return { ok: false, reason: "once" };
  }
  if (lower.startsWith("every:")) {
    const rest = s.slice("every:".length).trim();
    const m = rest.match(/^(\d+)([mh])$/i);
    if (!m || Number(m[1]) < 1) return { ok: false, reason: "invalid" };
    return { ok: true, kind: "interval" };
  }
  const parts = s.split(/\s+/);
  if (parts.length !== 5) return { ok: false, reason: "invalid" };
  const bounds: Array<[number, number]> = [
    [0, 59],
    [0, 23],
    [1, 31],
    [1, 12],
    [0, 7],
  ];
  for (let i = 0; i < 5; i++) {
    if (!cronFieldOk(parts[i], bounds[i][0], bounds[i][1])) return { ok: false, reason: "invalid" };
  }
  return { ok: true, kind: "five" };
}

function cronFieldOk(raw: string, min: number, max: number): boolean {
  const v = (raw || "").trim();
  if (v === "*") return true;
  const step = v.match(/^\*\/(\d+)$/);
  if (step) {
    const n = Number(step[1]);
    return n >= 1 && n <= max;
  }
  if (!/^\d+$/.test(v)) return false;
  const n = Number(v);
  return n >= min && n <= max;
}

export function cronCreateBlocked(jobs: PageState, sessions: PageState, sessionCount: number): boolean {
  if (inventoryBlocksMutation(jobs.kind)) return true;
  if (inventoryBlocksMutation(sessions.kind)) return true;
  if (sessions.kind === "loading" && sessionCount <= 0) return true;
  if (sessions.kind === "empty" || (sessions.kind === "ready" && sessionCount <= 0)) return true;
  return false;
}

export type ConnectorFormError = "needName" | "badTransport" | "needUrl" | "needCommand";

export function connectorFormError(form: { name?: string; transport?: string; endpoint?: string }): ConnectorFormError | null {
  if (!(form.name || "").trim()) return "needName";
  const transport = normalizeTransport(form.transport || "");
  if (!transport) return "badTransport";
  const endpoint = (form.endpoint || "").trim();
  if (!endpoint) return transport === "mcp-stdio" ? "needCommand" : "needUrl";
  if (transport === "http" || transport === "mcp-http") {
    if (!isHttpEndpoint(endpoint)) return "needUrl";
  }
  return null;
}

export function publicConnector(row: Record<string, unknown> | null | undefined): ConnectorInfo | null {
  if (!row || typeof row !== "object") return null;
  if (connectorRowLeaksSecret(row)) return null;
  const name = typeof row.name === "string" ? row.name.trim() : "";
  if (!name) return null;
  const transport = typeof row.transport === "string" ? row.transport : "";
  return {
    name,
    transport,
    endpoint: typeof row.endpoint === "string" ? row.endpoint : "",
    enabled: row.enabled !== false,
    health: typeof row.health === "string" ? row.health : undefined,
    health_error: typeof row.health_error === "string" ? row.health_error : undefined,
    token_set: row.token_set === true,
    source: typeof row.source === "string" ? row.source : undefined,
    env_owned: row.env_owned === true,
    env_set: row.env_set === true,
    credential_ref: safeCredentialRef(row.credential_ref),
  };
}

/** GET may expose an env-var name, never a token value. */
export function safeCredentialRef(raw: unknown): string | undefined {
  if (typeof raw !== "string") return undefined;
  const s = raw.trim();
  if (!s || s === "***") return undefined;
  if (!/^[A-Z][A-Z0-9_]*$/.test(s)) return undefined;
  return s;
}

export function connectorRowLeaksSecret(row: Record<string, unknown>): boolean {
  for (const [k, v] of Object.entries(row)) {
    if (SECRET_ROW_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.trim()) return true;
  }
  return false;
}

export function connectorTestReady(row: { name?: string; transport?: string; endpoint?: string } | null | undefined): boolean {
  if (!row || !(row.name || "").trim()) return false;
  return connectorFormError({ name: row.name, transport: row.transport || "http", endpoint: row.endpoint }) == null;
}

export function supportedTransports(): readonly ConnectorTransport[] {
  return CONNECTOR_TRANSPORTS;
}

export function disposeOneTimeSecrets(last: LastSecret | null | undefined): LastSecret | null {
  if (!last) return null;
  return { id: last.id, token_prefix: last.token_prefix, note: last.note };
}

export function confirmNamedTarget(message: string, confirmFn: ConfirmFn): boolean {
  return confirmNamed(message, confirmFn);
}

export function ttsItemCount(loaded: boolean): number {
  return loaded ? 1 : 0;
}

export function ttsBlocksMutation(kind: PageLoadKind): boolean {
  return inventoryBlocksMutation(kind) || kind === "loading";
}

export function isPermissionOrError(kind: PageLoadKind): boolean {
  return kind === "permission" || kind === "error";
}

export { isPermissionError, classifyPageState, inventoryBlocksMutation };

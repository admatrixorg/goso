export type ChannelRow = {
  name: string;
  configured: boolean;
  missing: boolean;
  env: string;
  env_names: string[];
  health?: string;
  transport?: string;
  secret_set?: boolean;
  from_env?: boolean;
  writable?: string[];
  bound_agent_id?: string;
  dm_policy?: string;
  group_policy?: string;
  require_mention?: boolean;
  allow_from?: string[];
  allow_from_count?: number;
  phase?: number;
  last_error?: string;
  enabled?: boolean;
};

export type ChannelPairingItem = {
  id: string;
  channel: string;
  sender_id: string;
  status: string;
  expires_at?: string;
};

export type ChannelHealthFilter = "" | "running" | "failed" | "missing" | "parked" | "stopped";

export const DM_POLICIES = ["open", "pairing", "allowlist", "disabled"] as const;
export const GROUP_POLICIES = ["open", "allowlist", "disabled"] as const;

export type ChannelRemediation = "ok" | "missing" | "failed" | "parked" | "stopped" | "from_env";

function asStringList(v: unknown): string[] {
  if (!Array.isArray(v)) return [];
  return v.filter((n): n is string => typeof n === "string" && n.length > 0);
}

export function normalizeChannelRow(c: Partial<ChannelRow> | null | undefined): ChannelRow | null {
  const name = typeof c?.name === "string" ? c.name.trim() : "";
  if (!name) return null;
  const env = typeof c?.env === "string" ? c.env : "";
  const envNames = asStringList(c?.env_names);
  return {
    name,
    configured: c?.configured === true,
    missing: c?.missing === true || c?.configured !== true,
    env,
    env_names: envNames.length ? envNames : env ? [env] : [],
    health: typeof c?.health === "string" ? c.health : "",
    transport: typeof c?.transport === "string" ? c.transport : "",
    secret_set: c?.secret_set === true,
    from_env: c?.from_env === true,
    writable: asStringList(c?.writable),
    bound_agent_id: typeof c?.bound_agent_id === "string" ? c.bound_agent_id : "",
    dm_policy: typeof c?.dm_policy === "string" ? c.dm_policy : "",
    group_policy: typeof c?.group_policy === "string" ? c.group_policy : "",
    require_mention: c?.require_mention === true,
    allow_from: asStringList(c?.allow_from),
    allow_from_count: typeof c?.allow_from_count === "number" ? c.allow_from_count : 0,
    phase: typeof c?.phase === "number" ? c.phase : 0,
    last_error: typeof c?.last_error === "string" ? c.last_error : "",
    enabled: c?.enabled === true,
  };
}

export function sanitizePairingItem(
  p: ChannelPairingItem & { code?: unknown; code_hash?: unknown },
): ChannelPairingItem {
  return {
    id: typeof p.id === "string" ? p.id : "",
    channel: typeof p.channel === "string" ? p.channel : "",
    sender_id: typeof p.sender_id === "string" ? p.sender_id : "",
    status: typeof p.status === "string" ? p.status : "",
    expires_at: typeof p.expires_at === "string" ? p.expires_at : undefined,
  };
}

export function pairingExposesCode(p: { code?: unknown; code_hash?: unknown }): boolean {
  const code = p.code == null ? "" : String(p.code);
  const hash = p.code_hash == null ? "" : String(p.code_hash);
  return code.trim().length > 0 || hash.trim().length > 0;
}

export function filterChannels<T extends { name: string; health?: string }>(
  rows: T[],
  opts: { query?: string; health?: ChannelHealthFilter } = {},
): T[] {
  const q = (opts.query || "").trim().toLowerCase();
  const health = (opts.health || "").trim();
  return rows.filter((c) => {
    if (health && (c.health || "") !== health) return false;
    if (!q) return true;
    return c.name.toLowerCase().includes(q);
  });
}

export function secretPutBody(writable: string[] | undefined, draft: Record<string, string>): Record<string, string> {
  const body: Record<string, string> = {};
  for (const field of writable ?? []) {
    const v = (draft[field] || "").trim();
    if (v) body[field] = v;
  }
  return body;
}

export function parseAllowFrom(raw: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of (raw || "").split(/[\s,]+/)) {
    const id = part.trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    out.push(id);
  }
  return out;
}

export function formatAllowFrom(ids?: string[]): string {
  return (ids ?? []).filter((id) => id.trim()).join("\n");
}

export function channelRemediation(c: {
  health?: string;
  last_error?: string;
  from_env?: boolean;
  missing?: boolean;
  phase?: number;
}): ChannelRemediation {
  if (c.phase === 2 || c.health === "parked") return "parked";
  if (c.health === "failed" || Boolean((c.last_error || "").trim())) return "failed";
  if (c.health === "missing" || c.missing) return "missing";
  if (c.from_env) return "from_env";
  if (c.health === "stopped") return "stopped";
  return "ok";
}

export function canClearBox(c: { writable?: string[]; secret_set?: boolean }): boolean {
  return (c.writable ?? []).length > 0 && c.secret_set === true;
}

export function isPhase2(c: { phase?: number; health?: string }): boolean {
  return c.phase === 2 || c.health === "parked";
}

export type CatalogTeam = { id: string; name: string; lead_agent_id?: string; members: number };
export type CatalogAgent = { id: string; agent_key: string; display_name: string; enabled: boolean; model?: string };
export type CatalogSkill = { name: string; path?: string };
export type CatalogMCP = { name: string; transport: string; endpoint?: string; enabled: boolean; token_set: boolean; env_owned: boolean };

export type Catalog = {
  teams: CatalogTeam[];
  agents: CatalogAgent[];
  skills: CatalogSkill[];
  mcp: CatalogMCP[];
  skills_configured: boolean;
  generated_at?: string;
};

export type ManifestItem = { kind: string; id?: string; key?: string; name: string };
export type Manifest = {
  schema_version: number;
  secret_policy: string;
  teams: ManifestItem[];
  agents: ManifestItem[];
  skills: ManifestItem[];
  mcp: ManifestItem[];
};

export type Archive = {
  schema: string;
  schema_version: number;
  exported_at?: string;
  include_secrets: boolean;
  manifest: Manifest;
  teams: unknown[];
  agents: unknown[];
  skills: unknown[];
  mcp: unknown[];
  warnings?: string[];
};

export type ReportItem = { kind: string; name: string; id?: string; detail?: string };
export type CredentialNeed = { kind: string; name: string; reason: string; env_name?: string };
export type Report = {
  created: ReportItem[];
  skipped: ReportItem[];
  overwritten: ReportItem[];
  renamed: ReportItem[];
  failed: ReportItem[];
  credentials_needed: CredentialNeed[];
};

export type Step = { name: string; status: string; detail?: string };

export type PortableJob = {
  id: string;
  kind: string;
  status: string;
  progress: number;
  dry_run: boolean;
  conflict?: string;
  steps: Step[];
  report: Report;
  archive?: Archive;
  error?: string;
  created_at?: string;
  updated_at?: string;
};

export type ConflictItem = { kind: string; name: string; existing?: string };

export type Preview = {
  valid: boolean;
  errors: string[];
  warnings: string[];
  manifest: Manifest;
  conflicts: ConflictItem[];
  archive?: Archive;
};

export type Selection = {
  team_ids: string[];
  agent_ids: string[];
  skill_names: string[];
  mcp_names: string[];
};

export type Conflict = "skip" | "overwrite" | "rename";

export const SCHEMA = "goso.portable/v1";
export const SCHEMA_VERSION = 1;

const SECRET_KEYS = new Set([
  "token",
  "secret",
  "password",
  "hmac",
  "hmac_key",
  "bot_token",
  "access_token",
  "api_key",
  "authorization",
  "private_key",
  "args",
  "arguments",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|gk_[0-9a-f]{16,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+|token=)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null) return false;
  if (typeof row === "string") return SECRET_VAL.test(row);
  if (Array.isArray(row)) return row.some(publicHasSecrets);
  if (typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    const lk = k.toLowerCase();
    if (SECRET_KEYS.has(lk) && typeof v === "string" && v.length > 0) return true;
    if (lk === "args" || lk === "arguments") return true;
    if (publicHasSecrets(v)) return true;
  }
  return false;
}

function str(v: unknown): string {
  return v == null ? "" : String(v);
}

function items(v: unknown): ManifestItem[] {
  if (!Array.isArray(v)) return [];
  const out: ManifestItem[] = [];
  for (const row of v) {
    if (!row || typeof row !== "object") continue;
    const r = row as Record<string, unknown>;
    const name = str(r.name || r.key).trim();
    if (!name) continue;
    const item: ManifestItem = { kind: str(r.kind), name };
    if (r.id) item.id = str(r.id);
    if (r.key) item.key = str(r.key);
    out.push(item);
  }
  return out;
}

export function emptyManifest(): Manifest {
  return { schema_version: SCHEMA_VERSION, secret_policy: "excluded", teams: [], agents: [], skills: [], mcp: [] };
}

export function emptyCatalog(): Catalog {
  return { teams: [], agents: [], skills: [], mcp: [], skills_configured: false };
}

export function emptyReport(): Report {
  return { created: [], skipped: [], overwritten: [], renamed: [], failed: [], credentials_needed: [] };
}

export function asPublicCatalog(j: Partial<Catalog> | null | undefined): Catalog {
  const teams = (j?.teams || []).filter((row) => row && !publicHasSecrets(row));
  const agents = (j?.agents || []).filter((row) => row && !publicHasSecrets(row));
  const skills = (j?.skills || []).filter((row) => row && !publicHasSecrets(row));
  const mcp = (j?.mcp || []).filter((row) => row && !publicHasSecrets(row));
  return {
    teams: teams.map((r) => ({ id: str(r.id), name: str(r.name), lead_agent_id: r.lead_agent_id ? str(r.lead_agent_id) : undefined, members: Number(r.members) || 0 })),
    agents: agents.map((r) => ({
      id: str(r.id),
      agent_key: str(r.agent_key),
      display_name: str(r.display_name),
      enabled: Boolean(r.enabled),
      model: r.model ? str(r.model) : undefined,
    })),
    skills: skills.map((r) => ({ name: str(r.name), path: r.path ? str(r.path) : undefined })),
    mcp: mcp.map((r) => ({
      name: str(r.name),
      transport: str(r.transport),
      endpoint: r.endpoint ? str(r.endpoint) : undefined,
      enabled: Boolean(r.enabled),
      token_set: Boolean(r.token_set),
      env_owned: Boolean(r.env_owned),
    })),
    skills_configured: Boolean(j?.skills_configured),
    generated_at: j?.generated_at ? str(j.generated_at) : undefined,
  };
}

export function asPublicArchive(j: Partial<Archive> | null | undefined): Archive | undefined {
  if (!j) return undefined;
  if (publicHasSecrets(j) || j.include_secrets) return undefined;
  return {
    schema: str(j.schema) || SCHEMA,
    schema_version: Number(j.schema_version) || SCHEMA_VERSION,
    exported_at: j.exported_at ? str(j.exported_at) : undefined,
    include_secrets: false,
    manifest: {
      schema_version: Number(j.manifest?.schema_version) || SCHEMA_VERSION,
      secret_policy: str(j.manifest?.secret_policy) || "excluded",
      teams: items(j.manifest?.teams),
      agents: items(j.manifest?.agents),
      skills: items(j.manifest?.skills),
      mcp: items(j.manifest?.mcp),
    },
    teams: Array.isArray(j.teams) ? j.teams : [],
    agents: Array.isArray(j.agents) ? j.agents : [],
    skills: Array.isArray(j.skills) ? j.skills : [],
    mcp: Array.isArray(j.mcp) ? j.mcp : [],
    warnings: Array.isArray(j.warnings) ? j.warnings.map(str) : [],
  };
}

export function asPublicJob(j: Partial<PortableJob> | null | undefined): PortableJob | undefined {
  if (!j || publicHasSecrets(j)) return undefined;
  const id = str(j.id).trim();
  if (!id) return undefined;
  return {
    id,
    kind: str(j.kind),
    status: str(j.status),
    progress: Number(j.progress) || 0,
    dry_run: Boolean(j.dry_run),
    conflict: j.conflict ? str(j.conflict) : undefined,
    steps: Array.isArray(j.steps) ? j.steps.map((s) => ({ name: str(s.name), status: str(s.status), detail: s.detail ? str(s.detail) : undefined })) : [],
    report: {
      created: j.report?.created || [],
      skipped: j.report?.skipped || [],
      overwritten: j.report?.overwritten || [],
      renamed: j.report?.renamed || [],
      failed: j.report?.failed || [],
      credentials_needed: j.report?.credentials_needed || [],
    },
    archive: asPublicArchive(j.archive),
    error: j.error ? str(j.error) : undefined,
    created_at: j.created_at ? str(j.created_at) : undefined,
    updated_at: j.updated_at ? str(j.updated_at) : undefined,
  };
}

export function asPublicPreview(j: Partial<Preview> | null | undefined): Preview {
  return {
    valid: Boolean(j?.valid),
    errors: Array.isArray(j?.errors) ? j!.errors.map(str) : [],
    warnings: Array.isArray(j?.warnings) ? j!.warnings.map(str) : [],
    manifest: j?.manifest
      ? {
          schema_version: Number(j.manifest.schema_version) || SCHEMA_VERSION,
          secret_policy: str(j.manifest.secret_policy) || "excluded",
          teams: items(j.manifest.teams),
          agents: items(j.manifest.agents),
          skills: items(j.manifest.skills),
          mcp: items(j.manifest.mcp),
        }
      : emptyManifest(),
    conflicts: Array.isArray(j?.conflicts) ? j!.conflicts : [],
    archive: asPublicArchive(j?.archive),
  };
}

export function selectionCount(sel: Selection): number {
  return sel.team_ids.length + sel.agent_ids.length + sel.skill_names.length + sel.mcp_names.length;
}

export function emptySelection(): Selection {
  return { team_ids: [], agent_ids: [], skill_names: [], mcp_names: [] };
}

export function toggleId(list: string[], id: string): string[] {
  const v = id.trim();
  if (!v) return list;
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
}

export function isConflict(s: string): s is Conflict {
  return s === "skip" || s === "overwrite" || s === "rename";
}

export function catalogHasSecrets(j: Partial<Catalog> | null | undefined): boolean {
  if (publicHasSecrets(j)) return true;
  return [...(j?.teams || []), ...(j?.agents || []), ...(j?.skills || []), ...(j?.mcp || [])].some(publicHasSecrets);
}

export function downloadJSON(filename: string, value: unknown): void {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function parseArchiveFile(text: string): unknown {
  return JSON.parse(text);
}

export function jobProgress(job: PortableJob | undefined): number {
  if (!job) return 0;
  if (job.status === "done" || job.status === "rolled_back") return 100;
  return Math.max(0, Math.min(100, job.progress || 0));
}

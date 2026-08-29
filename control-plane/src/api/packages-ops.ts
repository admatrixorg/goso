export const ECOSYSTEMS = ["system", "python", "node", "github"] as const;
export type Ecosystem = (typeof ECOSYSTEMS)[number];

export const CLI_KINDS = ["github", "npm", "pypi"] as const;
export type CLIKind = (typeof CLI_KINDS)[number];

export type Runtime = {
  name: string;
  ecosystem?: string;
  present: boolean;
  version?: string;
  compatible: boolean;
  warning?: string;
};

export type AllowEntry = {
  id: string;
  ecosystem: string;
  name: string;
  pin: string;
  created_at?: string;
};

export type Pkg = {
  id: string;
  ecosystem: string;
  name: string;
  version: string;
  status: string;
  warning?: string;
  job_id?: string;
  created_at?: string;
  updated_at?: string;
};

export type PkgJob = {
  id: string;
  action: string;
  package_id: string;
  ecosystem: string;
  name: string;
  version: string;
  status: string;
  progress: number;
  log: string[];
  error?: string;
  started_at?: string;
  finished_at?: string;
};

export type CLICred = {
  kind: string;
  set: boolean;
  updated_at?: string;
};

export type Snapshot = {
  runtimes: Runtime[];
  allowlist: AllowEntry[];
  packages: Pkg[];
  jobs: PkgJob[];
  credentials: CLICred[];
};

const SECRET_KEYS = new Set([
  "token",
  "secret",
  "password",
  "hmac",
  "hmac_key",
  "code",
  "bot_token",
  "access_token",
  "api_key",
  "hash",
  "key_hash",
  "authorization",
  "private_key",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+|npm_[A-Za-z0-9]+|token=)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
    if (Array.isArray(v) && v.some((x) => typeof x === "string" && SECRET_VAL.test(x))) return true;
  }
  return false;
}

export function isKnownEco(s: string): s is Ecosystem {
  return (ECOSYSTEMS as readonly string[]).includes(s);
}

export function isKnownKind(s: string): s is CLIKind {
  return (CLI_KINDS as readonly string[]).includes(s);
}

export function pinValid(pin: string): boolean {
  const v = (pin || "").trim();
  if (!v) return false;
  if (/^(latest|\*|x|any)$/i.test(v)) return false;
  if (/[~^<>*]|>=|<=/.test(v)) return false;
  return /^[A-Za-z0-9._+-]{1,64}$/.test(v);
}

export function asPublicRuntimes(rows: Runtime[] | null | undefined): Runtime[] {
  const out: Runtime[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const name = String(row.name || "").trim();
    if (!name) continue;
    out.push({
      name,
      ecosystem: row.ecosystem ? String(row.ecosystem) : undefined,
      present: Boolean(row.present),
      version: row.version ? String(row.version) : undefined,
      compatible: Boolean(row.compatible),
      warning: row.warning ? String(row.warning) : undefined,
    });
  }
  return out;
}

export function asPublicAllow(rows: AllowEntry[] | null | undefined): AllowEntry[] {
  const out: AllowEntry[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row) || !isKnownEco(String(row.ecosystem || ""))) continue;
    const id = String(row.id || "").trim();
    const name = String(row.name || "").trim();
    const pin = String(row.pin || "").trim();
    if (!id || !name || !pin) continue;
    out.push({
      id,
      ecosystem: String(row.ecosystem),
      name,
      pin,
      created_at: row.created_at ? String(row.created_at) : undefined,
    });
  }
  return out;
}

export function asPublicPackages(rows: Pkg[] | null | undefined): Pkg[] {
  const out: Pkg[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row) || !isKnownEco(String(row.ecosystem || ""))) continue;
    const id = String(row.id || "").trim();
    if (!id) continue;
    out.push({
      id,
      ecosystem: String(row.ecosystem),
      name: String(row.name || ""),
      version: String(row.version || ""),
      status: String(row.status || ""),
      warning: row.warning ? String(row.warning) : undefined,
      job_id: row.job_id ? String(row.job_id) : undefined,
      created_at: row.created_at ? String(row.created_at) : undefined,
      updated_at: row.updated_at ? String(row.updated_at) : undefined,
    });
  }
  return out;
}

export function asPublicJobs(rows: PkgJob[] | null | undefined): PkgJob[] {
  const out: PkgJob[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const id = String(row.id || "").trim();
    if (!id) continue;
    const log = Array.isArray(row.log)
      ? row.log.filter((l) => typeof l === "string" && !SECRET_VAL.test(l)).map((l) => String(l).slice(0, 240))
      : [];
    out.push({
      id,
      action: String(row.action || ""),
      package_id: String(row.package_id || ""),
      ecosystem: String(row.ecosystem || ""),
      name: String(row.name || ""),
      version: String(row.version || ""),
      status: String(row.status || ""),
      progress: Number.isFinite(Number(row.progress)) ? Math.max(0, Math.min(100, Number(row.progress))) : 0,
      log,
      error: row.error ? String(row.error) : undefined,
      started_at: row.started_at ? String(row.started_at) : undefined,
      finished_at: row.finished_at ? String(row.finished_at) : undefined,
    });
  }
  return out;
}

export function asPublicCreds(rows: CLICred[] | null | undefined): CLICred[] {
  const out: CLICred[] = [];
  for (const kind of CLI_KINDS) {
    const row = (rows || []).find((r) => r && String(r.kind) === kind);
    if (row && publicHasSecrets(row)) {
      out.push({ kind, set: false });
      continue;
    }
    out.push({
      kind,
      set: Boolean(row?.set),
      updated_at: row?.updated_at ? String(row.updated_at) : undefined,
    });
  }
  return out;
}

export function asPublicSnapshot(raw: Partial<Snapshot> | null | undefined): Snapshot {
  return {
    runtimes: asPublicRuntimes(raw?.runtimes),
    allowlist: asPublicAllow(raw?.allowlist),
    packages: asPublicPackages(raw?.packages),
    jobs: asPublicJobs(raw?.jobs),
    credentials: asPublicCreds(raw?.credentials),
  };
}

export function snapshotHasSecrets(raw: unknown): boolean {
  if (raw == null || typeof raw !== "object") return false;
  const rec = raw as Record<string, unknown>;
  if (publicHasSecrets(rec)) return true;
  for (const key of ["runtimes", "allowlist", "packages", "jobs", "credentials"]) {
    const rows = rec[key];
    if (Array.isArray(rows) && rows.some((r) => publicHasSecrets(r))) return true;
  }
  return false;
}

export function filterByEco<T extends { ecosystem: string; name?: string; id?: string }>(rows: T[], eco: string, q = ""): T[] {
  const ecoRows = rows.filter((r) => r.ecosystem === eco);
  const needle = q.trim().toLowerCase();
  if (!needle) return ecoRows;
  return ecoRows.filter((r) => `${r.id || ""} ${r.name || ""} ${r.ecosystem}`.toLowerCase().includes(needle));
}

export function runtimeForEco(runtimes: Runtime[], eco: string): Runtime | undefined {
  return runtimes.find((r) => r.ecosystem === eco) || runtimes.find((r) => r.name === eco);
}

export function jobsFor(jobs: PkgJob[], pkgId?: string, eco?: string): PkgJob[] {
  const rows = [...jobs].reverse();
  if (pkgId) return rows.filter((j) => j.package_id === pkgId);
  if (eco) return rows.filter((j) => j.ecosystem === eco);
  return rows;
}

export function latestJob(jobs: PkgJob[], pkgId?: string, eco?: string): PkgJob | undefined {
  return jobsFor(jobs, pkgId, eco)[0];
}

export function jobActive(job: PkgJob | undefined): boolean {
  if (!job) return false;
  return job.status === "queued" || job.status === "running";
}

export function pkgConfirmMatch(typed: string, row: Pick<Pkg, "id" | "name" | "ecosystem">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.name || v === `${row.ecosystem}/${row.name}`;
}

export function allowConfirmMatch(typed: string, row: Pick<AllowEntry, "id" | "name" | "ecosystem">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.name || v === `${row.ecosystem}/${row.name}`;
}

export function cliConfirmMatch(typed: string, kind: string): boolean {
  return (typed || "").trim() === kind;
}

export function pkgLabel(row: Pick<Pkg, "id" | "name" | "ecosystem" | "version">): string {
  const n = (row.name || "").trim();
  if (n && row.version) return `${row.ecosystem}/${n}@${row.version}`;
  return n || row.id;
}

export function formatWhen(iso: string | undefined, fallback: string): string {
  const s = (iso || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

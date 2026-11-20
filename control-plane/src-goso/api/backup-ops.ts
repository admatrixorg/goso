export type BackupScope = "system" | "tenant";
export type BackupDest = "local" | "s3";

export type BackupFile = {
  file: string;
  bytes: number;
  integrity: string;
  mtime?: string;
  scope?: string;
  tenant?: string;
  secret_policy?: string;
  destination?: string;
  remote_key?: string;
  progress?: number;
  steps?: BackupStep[];
  warning?: string;
  counts?: Record<string, number>;
};

export type BackupStep = { name: string; status: string; detail?: string };

export type BackupList = { files: BackupFile[] };

export type BackupCheck = { id: string; ok: boolean; blocking?: boolean; detail?: string };

export type Preflight = {
  engine: string;
  can_backup: boolean;
  can_restore: boolean;
  blocking?: string;
  checks: BackupCheck[];
};

export type RestorePlan = {
  valid: boolean;
  file: string;
  integrity: string;
  scope: string;
  tenant?: string;
  secret_policy: string;
  credentials_excluded: boolean;
  live_apply_cli_only: boolean;
  errors: string[];
  warnings: string[];
  archive_counts?: Record<string, number>;
  live_counts?: Record<string, number>;
  actions: string[];
  recovery: { strategy: string; pre_restore_suffix: string; temp_cleanup: boolean; live_apply_cli_only: boolean };
  confirm_required: boolean;
  confirm_target?: string;
};

export type S3Status = {
  configured: boolean;
  endpoint?: string;
  bucket?: string;
  region?: string;
  prefix?: string;
  access_key_set: boolean;
  env_owned?: boolean;
};

export type S3Write = {
  endpoint: string;
  bucket: string;
  region: string;
  prefix: string;
  access_key: string;
  secret: string;
};

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
  "access_key",
  "secret_access_key",
  "access_key_id",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+|token=)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
    if (v && typeof v === "object" && publicHasSecrets(v)) return true;
    if (Array.isArray(v) && v.some((x) => publicHasSecrets(x))) return true;
  }
  return false;
}

export function asPublicFile(row: unknown): BackupFile | undefined {
  if (!row || typeof row !== "object") return undefined;
  const rec = row as Record<string, unknown>;
  if (publicHasSecrets(rec)) return undefined;
  const file = String(rec.file || "");
  if (!file) return undefined;
  return {
    file,
    bytes: Number(rec.bytes) || 0,
    integrity: String(rec.integrity || ""),
    mtime: rec.mtime ? String(rec.mtime) : undefined,
    scope: rec.scope ? String(rec.scope) : undefined,
    tenant: rec.tenant ? String(rec.tenant) : undefined,
    secret_policy: rec.secret_policy ? String(rec.secret_policy) : undefined,
    destination: rec.destination ? String(rec.destination) : undefined,
    remote_key: rec.remote_key ? String(rec.remote_key) : undefined,
    progress: typeof rec.progress === "number" ? rec.progress : undefined,
    warning: rec.warning ? String(rec.warning) : undefined,
    counts: rec.counts && typeof rec.counts === "object" ? (rec.counts as Record<string, number>) : undefined,
    steps: Array.isArray(rec.steps)
      ? (rec.steps as BackupStep[]).map((s) => ({ name: String(s.name || ""), status: String(s.status || ""), detail: s.detail ? String(s.detail) : undefined }))
      : undefined,
  };
}

export function asPublicList(row: unknown): BackupList {
  const rec = row && typeof row === "object" ? (row as { files?: unknown[] }) : {};
  const files = Array.isArray(rec.files) ? rec.files.map(asPublicFile).filter((x): x is BackupFile => x != null) : [];
  return { files };
}

export function asPublicPreflight(row: unknown): Preflight {
  const rec = row && typeof row === "object" ? (row as Record<string, unknown>) : {};
  const checks = Array.isArray(rec.checks)
    ? (rec.checks as BackupCheck[]).filter((c) => c && !publicHasSecrets(c)).map((c) => ({
        id: String(c.id || ""),
        ok: Boolean(c.ok),
        blocking: Boolean(c.blocking),
        detail: c.detail ? String(c.detail) : undefined,
      }))
    : [];
  return {
    engine: String(rec.engine || ""),
    can_backup: Boolean(rec.can_backup),
    can_restore: Boolean(rec.can_restore),
    blocking: rec.blocking ? String(rec.blocking) : undefined,
    checks,
  };
}

export function asPublicPlan(row: unknown): RestorePlan | undefined {
  if (!row || typeof row !== "object" || publicHasSecrets(row)) return undefined;
  const rec = row as Record<string, unknown>;
  return {
    valid: Boolean(rec.valid),
    file: String(rec.file || ""),
    integrity: String(rec.integrity || ""),
    scope: String(rec.scope || ""),
    tenant: rec.tenant ? String(rec.tenant) : undefined,
    secret_policy: String(rec.secret_policy || "excluded"),
    credentials_excluded: rec.credentials_excluded !== false,
    live_apply_cli_only: rec.live_apply_cli_only !== false,
    errors: Array.isArray(rec.errors) ? rec.errors.map(String) : [],
    warnings: Array.isArray(rec.warnings) ? rec.warnings.map(String) : [],
    archive_counts: rec.archive_counts && typeof rec.archive_counts === "object" ? (rec.archive_counts as Record<string, number>) : undefined,
    live_counts: rec.live_counts && typeof rec.live_counts === "object" ? (rec.live_counts as Record<string, number>) : undefined,
    actions: Array.isArray(rec.actions) ? rec.actions.map(String) : [],
    recovery: {
      strategy: "pre_restore_rename",
      pre_restore_suffix: ".pre-restore",
      temp_cleanup: true,
      live_apply_cli_only: true,
    },
    confirm_required: rec.confirm_required !== false,
    confirm_target: rec.confirm_target ? String(rec.confirm_target) : undefined,
  };
}

export function asPublicS3(row: unknown): S3Status | undefined {
  if (!row || typeof row !== "object" || publicHasSecrets(row)) return undefined;
  const rec = row as Record<string, unknown>;
  return {
    configured: Boolean(rec.configured),
    endpoint: rec.endpoint ? String(rec.endpoint) : undefined,
    bucket: rec.bucket ? String(rec.bucket) : undefined,
    region: rec.region ? String(rec.region) : undefined,
    prefix: rec.prefix ? String(rec.prefix) : undefined,
    access_key_set: Boolean(rec.access_key_set),
    env_owned: Boolean(rec.env_owned),
  };
}

export function emptyS3(): S3Status {
  return { configured: false, access_key_set: false };
}

export function emptyPreflight(): Preflight {
  return { engine: "", can_backup: false, can_restore: false, checks: [] };
}

export function confirmMatches(file: string, typed: string): boolean {
  return file.trim() !== "" && typed.trim() === file.trim();
}

export function filterByScope(files: BackupFile[], scope: BackupScope): BackupFile[] {
  return files.filter((f) => (f.scope || "system") === scope);
}

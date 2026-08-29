import type { StorageCrumb, StorageEntry, StorageListing, StoragePreview } from "./storage";

export const PREVIEW_CAP = 64 * 1024;

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
  "private_key",
  "ssh_key",
  "pem",
  "key",
  "content",
]);

const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|BEGIN [A-Z ]*PRIVATE KEY)/i;

const SECRET_NAME =
  /(^|\/)(\.env(\.|$)|id_rsa|id_dsa|id_ecdsa|id_ed25519|secrets|credentials|runtime)(\/|$)/i;
const SECRET_EXT = /\.(pem|key|p12|pfx|crt|cer|ppk|p8|der)$/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    const key = k.toLowerCase();
    if (SECRET_KEYS.has(key) && typeof v === "string" && v.length > 0) {
      if (key === "content" || key === "text") return true;
      return true;
    }
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

export function secretPath(path: string): boolean {
  const p = path.replace(/\\/g, "/");
  return SECRET_NAME.test(p) || SECRET_EXT.test(p);
}

export function asPublicListing(raw: StorageListing | null | undefined): StorageListing {
  const empty: StorageListing = {
    configured: false,
    path: "",
    breadcrumbs: [{ name: "workspace", path: "" }],
    entries: [],
    used_bytes: 0,
    max_bytes: 0,
    hidden_skipped: 0,
  };
  if (!raw || typeof raw !== "object") return empty;
  const entries: StorageEntry[] = [];
  for (const e of raw.entries || []) {
    if (!e || typeof e !== "object") continue;
    if (publicHasSecrets(e) || secretPath(e.path || e.name || "")) continue;
    const name = String(e.name || "").trim();
    const path = String(e.path || "").trim();
    if (!name) continue;
    entries.push({
      name,
      path,
      dir: Boolean(e.dir),
      size: Number(e.size) || 0,
      type: e.type ? String(e.type) : "",
      mtime: e.mtime ? String(e.mtime) : undefined,
    });
  }
  const crumbs: StorageCrumb[] = [];
  for (const c of raw.breadcrumbs || []) {
    if (!c || typeof c !== "object") continue;
    crumbs.push({ name: String(c.name || ""), path: String(c.path || "") });
  }
  if (crumbs.length === 0) crumbs.push({ name: "workspace", path: "" });
  return {
    configured: raw.configured === true,
    path: String(raw.path || ""),
    parent: raw.parent ? String(raw.parent) : "",
    breadcrumbs: crumbs,
    entries,
    used_bytes: Number(raw.used_bytes) || 0,
    max_bytes: Number(raw.max_bytes) || 0,
    hidden_skipped: Number(raw.hidden_skipped) || 0,
    truncated: Boolean(raw.truncated),
  };
}

export function asPublicPreview(raw: StoragePreview | null | undefined): StoragePreview | null {
  if (!raw || typeof raw !== "object") return null;
  if (publicHasSecrets(raw) || secretPath(raw.path || "")) return null;
  if (raw.kind === "denied") {
    return { path: String(raw.path || ""), type: String(raw.type || ""), size: Number(raw.size) || 0, kind: "denied", bytes: 0 };
  }
  let text = raw.text ? String(raw.text) : "";
  if (SECRET_VAL.test(text)) return { path: String(raw.path || ""), type: String(raw.type || ""), size: Number(raw.size) || 0, kind: "denied", bytes: 0 };
  if (text.length > PREVIEW_CAP) text = `${text.slice(0, PREVIEW_CAP)}…`;
  return {
    path: String(raw.path || ""),
    type: String(raw.type || ""),
    size: Number(raw.size) || 0,
    kind: raw.kind === "text" ? "text" : raw.kind === "binary" ? "binary" : String(raw.kind || "binary"),
    text: raw.kind === "text" ? text : undefined,
    truncated: Boolean(raw.truncated) || (raw.kind === "text" && text.length >= PREVIEW_CAP),
    bytes: Number(raw.bytes) || 0,
  };
}

export function storageConfirmMatch(typed: string, row: { name: string; path: string }): boolean {
  const got = typed.trim();
  if (!got) return false;
  return got === row.name || got === row.path;
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MiB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GiB`;
}

export function formatWhen(raw?: string, fallback = "—"): string {
  const s = (raw || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toISOString().replace("T", " ").replace(/\.\d+Z$/, " UTC");
}

export function isImageType(type?: string): boolean {
  return Boolean(type && type.startsWith("image/"));
}

export function quotaOver(used: number, max: number): boolean {
  return max > 0 && used >= max;
}

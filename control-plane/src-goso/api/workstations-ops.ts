import type { Workstation, WorkstationTest, WorkstationWrite } from "./workstations";

const SECRET_KEYS = new Set([
  "token",
  "secret",
  "password",
  "hmac",
  "hmac_key",
  "code",
  "bot_token",
  "access_token",
  "content",
  "text",
  "body",
  "api_key",
  "private_key",
  "ssh_key",
  "pem",
  "key",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|BEGIN [A-Z ]*PRIVATE KEY)/i;

export function looksLikeKey(s: string): boolean {
  const v = (s || "").trim();
  if (!v) return false;
  const u = v.toUpperCase();
  if (u.includes("PRIVATE KEY") || v.includes("-----") || /BEGIN /.test(u)) return true;
  if (/[\n\r]/.test(v)) return true;
  if (v.length > 200 && !/[/~]/.test(v)) return true;
  return false;
}

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    const key = k.toLowerCase();
    if (SECRET_KEYS.has(key) && typeof v === "string" && v.length > 0) return true;
    if (key === "identity_ref" && typeof v === "string" && looksLikeKey(v)) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

export function asPublic(rows: Workstation[] | null | undefined): Workstation[] {
  const out: Workstation[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const identityRef = String(row.identity_ref || "");
    if (looksLikeKey(identityRef)) continue;
    out.push({
      id: String(row.id || ""),
      display: String(row.display || ""),
      backend: String(row.backend || ""),
      host: String(row.host || ""),
      port: Number(row.port) || 0,
      user: row.user ? String(row.user) : undefined,
      identity_ref: identityRef || undefined,
      identity_set: Boolean(row.identity_set) || Boolean(identityRef),
      agent_id: row.agent_id ? String(row.agent_id) : undefined,
      health: String(row.health || ""),
      last_tested: row.last_tested ? String(row.last_tested) : undefined,
      created_at: row.created_at ? String(row.created_at) : undefined,
      updated_at: row.updated_at ? String(row.updated_at) : undefined,
    });
  }
  return out;
}

export function asPublicTest(row: WorkstationTest | null | undefined): WorkstationTest | null {
  if (!row || publicHasSecrets(row)) return null;
  return {
    ok: Boolean(row.ok),
    health: String(row.health || ""),
    summary: String(row.summary || "").slice(0, 160),
    backend: String(row.backend || ""),
    host: String(row.host || ""),
    port: Number(row.port) || 0,
    identity_set: Boolean(row.identity_set),
  };
}

export function wsConfirmMatch(typed: string, row: Pick<Workstation, "id" | "display">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.display;
}

export function wsLabel(row: Pick<Workstation, "id" | "display">): string {
  return (row.display || "").trim() || row.id;
}

export function formatWhen(iso: string | undefined, fallback: string): string {
  const s = (iso || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

export function writeBody(form: {
  display: string;
  backend: string;
  host: string;
  port: string;
  user: string;
  identity_ref: string;
  agent_id: string;
}): WorkstationWrite {
  const port = Number.parseInt(form.port, 10);
  const body: WorkstationWrite = {
    display: form.display.trim(),
    backend: form.backend.trim(),
    host: form.host.trim(),
  };
  if (Number.isFinite(port) && port > 0) body.port = port;
  if (form.user.trim()) body.user = form.user.trim();
  if (form.identity_ref.trim()) body.identity_ref = form.identity_ref.trim();
  if (form.agent_id.trim()) body.agent_id = form.agent_id.trim();
  return body;
}

export function identityError(identityRef: string): "ws.needPath" | "ws.keyMaterial" | null {
  const v = identityRef.trim();
  if (!v) return null;
  if (looksLikeKey(v)) return "ws.keyMaterial";
  if (v.includes("://")) return "ws.needPath";
  return null;
}

export type WsFormError = "ws.needDisplay" | "ws.needHost" | "ws.needBackend" | "ws.needPath" | "ws.keyMaterial";

/** Client field checks. Distinct from POST /test, which only validates stored config. */
export function wsFormError(form: {
  display: string;
  backend: string;
  host: string;
  identity_ref: string;
}): WsFormError | null {
  if (!(form.display || "").trim()) return "ws.needDisplay";
  if (!(form.backend || "").trim()) return "ws.needBackend";
  if (!(form.host || "").trim()) return "ws.needHost";
  return identityError(form.identity_ref);
}

/** Workstation test never claims a live SSH/Docker session. */
export function testOutcome(row: WorkstationTest | null | undefined): "none" | "valid" | "invalid" {
  if (!row) return "none";
  return row.ok ? "valid" : "invalid";
}

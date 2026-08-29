export const CONNECTOR_TRANSPORTS = ["http", "mcp-http", "mcp-stdio"] as const;
export type ConnectorTransport = (typeof CONNECTOR_TRANSPORTS)[number];

export type ConnectorInfo = {
  name: string;
  transport: string;
  endpoint: string;
  enabled: boolean;
  health?: string;
  health_error?: string;
  token_set?: boolean;
  source?: string;
  env_owned?: boolean;
  env_set?: boolean;
  credential_ref?: string;
};

export type AgentToolInfo = {
  name: string;
  connector: string;
  description?: string;
  requires_approval: boolean;
  enabled: boolean;
  configured?: boolean;
  granted?: boolean;
};

export type ConnectorTestResult = {
  ok: boolean;
  latency_ms: number;
  health?: string;
  error?: string;
};

export type ConnectorTestView = {
  ok: boolean;
  latency_ms: number;
  health: string;
  error: string;
};

export type ConnectorWrite = {
  name?: string;
  transport?: string;
  endpoint?: string;
  token?: string;
  credential_ref?: string;
  enabled?: boolean;
};

/** Map operator labels onto stored transports: http, mcp-http (SSE), mcp-stdio. */
export function normalizeTransport(raw: string): ConnectorTransport | "" {
  const v = raw.trim().toLowerCase();
  if (v === "http" || v === "mcp") return "http";
  if (v === "mcp-http" || v === "sse" || v === "mcp-sse" || v === "streamable-http") return "mcp-http";
  if (v === "mcp-stdio" || v === "stdio") return "mcp-stdio";
  return "";
}

export function isConnectorEnvOwned(c: { source?: string; env_owned?: boolean }): boolean {
  return c.env_owned === true || c.source === "env";
}

export function isConnectorEnabled(c: { enabled?: boolean }): boolean {
  return c.enabled !== false;
}

export function configuredLabel(configured?: boolean): "configured" | "not_configured" {
  return configured ? "configured" : "not_configured";
}

export function connectorWriteBody(form: ConnectorWrite): ConnectorWrite {
  const out: ConnectorWrite = {};
  if (form.name != null) out.name = form.name.trim();
  const transport = normalizeTransport(form.transport || "");
  if (transport) out.transport = transport;
  if (form.endpoint != null) out.endpoint = form.endpoint.trim();
  if (form.enabled != null) out.enabled = form.enabled;
  const tok = (form.token || "").trim();
  if (tok) out.token = tok;
  const cred = (form.credential_ref || "").trim();
  if (cred) out.credential_ref = cred;
  return out;
}

function redactTestText(s: string): string {
  let out = s
    .replace(/"(authorization|api[_-]?key|secret|token|password)"\s*:\s*"(?:\\.|[^"\\])*"/gi, '"$1":"[redacted]"')
    .replace(/Bearer\s+[^\s"'\\]+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_*-]+\b/g, "sk-[redacted]");
  if (out.length > 400) out = `${out.slice(0, 400)}…`;
  return out;
}

export function formatConnectorTest(raw: ConnectorTestResult & Record<string, unknown>): ConnectorTestView {
  const error = redactTestText(typeof raw.error === "string" ? raw.error : "");
  return {
    ok: raw.ok === true,
    latency_ms: typeof raw.latency_ms === "number" ? raw.latency_ms : 0,
    health: typeof raw.health === "string" ? raw.health : raw.ok ? "ok" : "unavailable",
    error,
  };
}

export function toolListLeaksSecret(row: Record<string, unknown>): boolean {
  if (typeof row.token === "string" && row.token.trim()) return true;
  if (typeof row.api_key === "string" && row.api_key.trim()) return true;
  return false;
}

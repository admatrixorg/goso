/** Environment-based configuration loader for GOSO MCP server */

export interface Config {
  /** GOSO gateway server URL (required) */
  goClawServer: string;
  /** Bearer token for authentication */
  goClawToken: string | undefined;
  /** Default user ID for multi-tenant scoping */
  goClawUserId: string | undefined;
  /** HTTP transport port (default: 3100) */
  mcpPort: number;
  /** Allowed origins for CORS (default: localhost) */
  allowedOrigins: string[];
  /** Rate limit: requests per minute per session (default: 60) */
  rateLimitRpm: number;
  /** Log level (default: info) */
  logLevel: "debug" | "info" | "warn" | "error";
}

const LOG_LEVELS = ["debug", "info", "warn", "error"] as const;

function firstEnv(...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = process.env[key];
    if (value) return value;
  }
  return undefined;
}

/**
 * Load configuration from environment variables.
 * Prefers GOSO_* (rebrand); falls back to GOCLAW_* for compatibility.
 * Throws if the gateway URL is missing.
 */
export function loadConfig(): Config {
  const goClawServer = firstEnv("GOSO_GATEWAY_URL", "GOCLAW_SERVER");
  if (!goClawServer) {
    throw new Error(
      "GOSO_GATEWAY_URL environment variable is required. " +
        "Set it to your GOSO gateway URL (e.g. http://localhost:8080)",
    );
  }

  const logLevel = (firstEnv("GOSO_LOG_LEVEL", "GOCLAW_LOG_LEVEL") ?? "info") as Config["logLevel"];
  if (!LOG_LEVELS.includes(logLevel)) {
    throw new Error(
      `Invalid GOSO_LOG_LEVEL: "${logLevel}". Must be one of: ${LOG_LEVELS.join(", ")}`,
    );
  }

  const mcpPort = parseInt(firstEnv("GOSO_MCP_PORT", "GOCLAW_MCP_PORT") ?? "3100", 10);
  if (isNaN(mcpPort) || mcpPort < 1 || mcpPort > 65535) {
    throw new Error(`Invalid GOSO_MCP_PORT: must be 1-65535`);
  }

  const allowedOrigins = (
    firstEnv("GOSO_MCP_ALLOWED_ORIGINS", "GOCLAW_MCP_ALLOWED_ORIGINS") ??
    "localhost,127.0.0.1,::1"
  )
    .split(",")
    .map((o) => o.trim())
    .filter(Boolean);

  const rateLimitRpm = parseInt(
    firstEnv("GOSO_MCP_RATE_LIMIT_RPM", "GOCLAW_MCP_RATE_LIMIT_RPM") ?? "60",
    10,
  );

  return {
    goClawServer: goClawServer.replace(/\/+$/, ""), // strip trailing slash
    goClawToken: firstEnv("GOSO_TOKEN", "GOCLAW_TOKEN"),
    goClawUserId: firstEnv("GOSO_USER_ID", "GOCLAW_USER_ID"),
    mcpPort,
    allowedOrigins,
    rateLimitRpm,
    logLevel,
  };
}

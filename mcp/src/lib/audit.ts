/**
 * Audit logger — structured logging for every tool invocation.
 * Records who/what/when/result for enterprise compliance.
 */

import { logger } from "./logger.js";

interface AuditEntry {
  tool: string;
  params: Record<string, unknown>;
  sessionId?: string;
  userId?: string;
  durationMs: number;
  success: boolean;
  error?: string;
}

/** Scrub sensitive fields from params before logging */
const SENSITIVE_KEYS = new Set([
  "api_key",
  "token",
  "password",
  "secret",
  "content",
  "config",
]);

function scrubParams(params: Record<string, unknown>): Record<string, unknown> {
  const scrubbed: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(params)) {
    if (SENSITIVE_KEYS.has(key.toLowerCase())) {
      scrubbed[key] = "[REDACTED]";
    } else if (typeof value === "string" && value.length > 200) {
      scrubbed[key] = value.substring(0, 200) + "...[truncated]";
    } else {
      scrubbed[key] = value;
    }
  }
  return scrubbed;
}

export function auditToolInvocation(entry: AuditEntry): void {
  const level = entry.success ? "info" : "warn";
  const data = {
    event: "tool_invoked",
    tool: entry.tool,
    params: scrubParams(entry.params),
    sessionId: entry.sessionId,
    userId: entry.userId,
    durationMs: entry.durationMs,
    result: entry.success ? "success" : "error",
    ...(entry.error ? { error: entry.error } : {}),
  };

  logger[level](`Tool: ${entry.tool}`, data);
}

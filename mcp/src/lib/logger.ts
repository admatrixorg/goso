/**
 * Structured JSON logger that writes to stderr (stdio-safe).
 * Never logs secrets (tokens, API keys, passwords).
 */

type LogLevel = "debug" | "info" | "warn" | "error";

const LEVEL_PRIORITY: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

/** Patterns that indicate sensitive data — scrub these from log output */
const SECRET_PATTERNS = [
  /Bearer\s+\S+/gi,
  /sk-[a-zA-Z0-9_-]{20,}/g,
  /key-[a-zA-Z0-9_-]{20,}/g,
  /ghp_[a-zA-Z0-9]{36,}/g,
  /gho_[a-zA-Z0-9]{36,}/g,
  /xoxb-[a-zA-Z0-9-]+/g,
  /password\s*[:=]\s*\S+/gi,
  /token\s*[:=]\s*\S+/gi,
  /api[_-]?key\s*[:=]\s*\S+/gi,
];

function scrubSecrets(text: string): string {
  let result = text;
  for (const pattern of SECRET_PATTERNS) {
    result = result.replace(pattern, "[REDACTED]");
  }
  return result;
}

class Logger {
  private minLevel: LogLevel = "info";

  setLevel(level: LogLevel): void {
    this.minLevel = level;
  }

  debug(message: string, data?: Record<string, unknown>): void {
    this.log("debug", message, data);
  }

  info(message: string, data?: Record<string, unknown>): void {
    this.log("info", message, data);
  }

  warn(message: string, data?: Record<string, unknown>): void {
    this.log("warn", message, data);
  }

  error(message: string, data?: Record<string, unknown>): void {
    this.log("error", message, data);
  }

  private log(level: LogLevel, message: string, data?: Record<string, unknown>): void {
    if (LEVEL_PRIORITY[level] < LEVEL_PRIORITY[this.minLevel]) return;

    const entry = {
      timestamp: new Date().toISOString(),
      level,
      message: scrubSecrets(message),
      ...(data ? { data: JSON.parse(scrubSecrets(JSON.stringify(data))) } : {}),
    };

    // Write to stderr so it doesn't interfere with stdio MCP transport
    process.stderr.write(JSON.stringify(entry) + "\n");
  }
}

/** Singleton logger instance */
export const logger = new Logger();

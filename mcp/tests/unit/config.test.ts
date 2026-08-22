import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { loadConfig } from "../../src/config.js";

describe("loadConfig", () => {
  const originalEnv = { ...process.env };

  function clearBrandEnv() {
    for (const key of Object.keys(process.env)) {
      if (key.startsWith("GOSO_") || key.startsWith("GOCLAW_")) delete process.env[key];
    }
  }

  beforeEach(() => {
    clearBrandEnv();
  });

  afterEach(() => {
    process.env = { ...originalEnv };
  });

  it("throws when GOSO_GATEWAY_URL is missing", () => {
    expect(() => loadConfig()).toThrow("GOSO_GATEWAY_URL");
  });

  it("loads with required GOSO_GATEWAY_URL only", () => {
    process.env.GOSO_GATEWAY_URL = "http://localhost:8080";
    const config = loadConfig();
    expect(config.goClawServer).toBe("http://localhost:8080");
    expect(config.goClawToken).toBeUndefined();
    expect(config.goClawUserId).toBeUndefined();
    expect(config.mcpPort).toBe(3100);
    expect(config.logLevel).toBe("info");
    expect(config.rateLimitRpm).toBe(60);
  });

  it("strips trailing slash from server URL", () => {
    process.env.GOSO_GATEWAY_URL = "http://localhost:8080///";
    const config = loadConfig();
    expect(config.goClawServer).toBe("http://localhost:8080");
  });

  it("loads all GOSO_* env vars", () => {
    process.env.GOSO_GATEWAY_URL = "https://goso.example.com";
    process.env.GOSO_TOKEN = "my-token";
    process.env.GOSO_USER_ID = "user-123";
    process.env.GOSO_MCP_PORT = "4000";
    process.env.GOSO_LOG_LEVEL = "debug";
    process.env.GOSO_MCP_ALLOWED_ORIGINS = "example.com,foo.bar";
    process.env.GOSO_MCP_RATE_LIMIT_RPM = "120";

    const config = loadConfig();
    expect(config.goClawServer).toBe("https://goso.example.com");
    expect(config.goClawToken).toBe("my-token");
    expect(config.goClawUserId).toBe("user-123");
    expect(config.mcpPort).toBe(4000);
    expect(config.logLevel).toBe("debug");
    expect(config.allowedOrigins).toEqual(["example.com", "foo.bar"]);
    expect(config.rateLimitRpm).toBe(120);
  });

  it("falls back to GOCLAW_* when GOSO_* is unset", () => {
    process.env.GOCLAW_SERVER = "http://localhost:9090";
    process.env.GOCLAW_TOKEN = "legacy-token";
    process.env.GOCLAW_MCP_PORT = "3200";
    const config = loadConfig();
    expect(config.goClawServer).toBe("http://localhost:9090");
    expect(config.goClawToken).toBe("legacy-token");
    expect(config.mcpPort).toBe(3200);
  });

  it("prefers GOSO_* over GOCLAW_*", () => {
    process.env.GOSO_GATEWAY_URL = "http://goso.local:8080";
    process.env.GOCLAW_SERVER = "http://goclaw.local:8080";
    const config = loadConfig();
    expect(config.goClawServer).toBe("http://goso.local:8080");
  });

  it("rejects invalid log level", () => {
    process.env.GOSO_GATEWAY_URL = "http://localhost:8080";
    process.env.GOSO_LOG_LEVEL = "verbose";
    expect(() => loadConfig()).toThrow("GOSO_LOG_LEVEL");
  });

  it("rejects invalid port", () => {
    process.env.GOSO_GATEWAY_URL = "http://localhost:8080";
    process.env.GOSO_MCP_PORT = "99999";
    expect(() => loadConfig()).toThrow("GOSO_MCP_PORT");
  });
});

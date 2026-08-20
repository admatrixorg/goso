import { type AddressInfo } from "node:net";
import { afterEach, describe, expect, it } from "vitest";
import { type Config } from "../../src/config.js";
import { createMcpHttpServer, SERVER_VERSION, type McpHttpServer } from "../../src/http.js";

function testConfig(): Config {
  return {
    goClawServer: "http://localhost:8080",
    goClawToken: undefined,
    goClawUserId: undefined,
    mcpPort: 0,
    allowedOrigins: ["localhost"],
    rateLimitRpm: 60,
    logLevel: "error",
  };
}

async function listen(app: McpHttpServer): Promise<number> {
  await new Promise<void>((resolve, reject) => {
    app.httpServer.once("error", reject);
    app.httpServer.listen(0, "127.0.0.1", () => resolve());
  });
  return (app.httpServer.address() as AddressInfo).port;
}

describe("GET /health", () => {
  let app: McpHttpServer | undefined;

  afterEach(async () => {
    if (app) {
      await app.shutdown();
      app = undefined;
    }
  });

  it("returns 200 with status ok and version", async () => {
    app = createMcpHttpServer(testConfig());
    const port = await listen(app);

    const res = await fetch(`http://127.0.0.1:${port}/health`);
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toContain("application/json");

    const body = await res.json();
    expect(body).toEqual({ status: "ok", version: SERVER_VERSION });
  });
});

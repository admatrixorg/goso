/**
 * GOSO MCP Server — Streamable HTTP transport entry point.
 * Used for production deployments supporting multiple concurrent clients.
 */

import { randomUUID } from "node:crypto";
import { createServer as createHttpServer, type Server as HttpServer } from "node:http";
import { pathToFileURL } from "node:url";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { loadConfig, type Config } from "./config.js";
import { createServer } from "./server.js";
import { logger } from "./lib/logger.js";
import { RateLimiter } from "./lib/rate-limiter.js";
import { SERVER_NAME, SERVER_VERSION } from "./version.js";

export { SERVER_NAME, SERVER_VERSION };

export interface McpHttpServer {
  httpServer: HttpServer;
  shutdown(): Promise<void>;
}

/**
 * Create the Streamable HTTP MCP server (does not listen).
 * GET /health is handled before MCP session routing.
 */
export function createMcpHttpServer(config: Config): McpHttpServer {
  const sessions = new Map<string, { server: McpServer; transport: StreamableHTTPServerTransport }>();
  const rateLimiter = new RateLimiter(config.rateLimitRpm);

  const httpServer = createHttpServer(async (req, res) => {
    const url = new URL(req.url ?? "/", `http://${req.headers.host}`);

    // Health check endpoint
    if (url.pathname === "/health") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ status: "ok", version: SERVER_VERSION }));
      return;
    }

    // Only handle /mcp path
    if (url.pathname !== "/mcp") {
      res.writeHead(404, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "Not found" }));
      return;
    }

    // Origin validation
    const origin = req.headers.origin;
    if (origin) {
      try {
        const originHost = new URL(origin).hostname;
        if (!config.allowedOrigins.includes(originHost)) {
          logger.warn("Blocked request from disallowed origin", { origin });
          res.writeHead(403, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "Origin not allowed" }));
          return;
        }
      } catch {
        res.writeHead(400, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "Invalid origin" }));
        return;
      }
    }

    // Read request body for POST
    let body: string | undefined;
    if (req.method === "POST") {
      const chunks: Buffer[] = [];
      for await (const chunk of req) {
        chunks.push(chunk as Buffer);
        // 1MB limit
        if (chunks.reduce((sum, c) => sum + c.length, 0) > 1_048_576) {
          res.writeHead(413, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "Request too large" }));
          return;
        }
      }
      body = Buffer.concat(chunks).toString("utf-8");
    }

    // Get or create session
    const sessionId = req.headers["mcp-session-id"] as string | undefined;

    if (sessionId && sessions.has(sessionId)) {
      // Rate limiting per session
      if (!rateLimiter.allow(sessionId)) {
        const retryAfter = rateLimiter.retryAfter(sessionId);
        logger.warn("Rate limited", { sessionId, retryAfter });
        res.writeHead(429, {
          "Content-Type": "application/json",
          "Retry-After": String(retryAfter),
        });
        res.end(JSON.stringify({ error: "Too many requests", retry_after: retryAfter }));
        return;
      }

      // Existing session — delegate to its transport
      const session = sessions.get(sessionId)!;
      await session.transport.handleRequest(req, res, body ? JSON.parse(body) : undefined);
      return;
    }

    if (req.method === "POST" && !sessionId) {
      // New session — create server + transport
      const transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(),
      });

      const server = createServer(config);
      await server.connect(transport);

      // Store session after connect (transport now has sessionId)
      const newSessionId = transport.sessionId;
      if (newSessionId) {
        sessions.set(newSessionId, { server, transport });
        logger.info("New MCP session created", { sessionId: newSessionId });

        // Clean up on close
        transport.onclose = () => {
          sessions.delete(newSessionId);
          rateLimiter.remove(newSessionId);
          logger.info("MCP session closed", { sessionId: newSessionId });
        };
      }

      await transport.handleRequest(req, res, body ? JSON.parse(body) : undefined);
      return;
    }

    if (req.method === "DELETE" && sessionId) {
      // Session termination
      const session = sessions.get(sessionId);
      if (session) {
        await session.transport.handleRequest(req, res);
        sessions.delete(sessionId);
        logger.info("MCP session terminated", { sessionId });
      } else {
        res.writeHead(404, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "Session not found" }));
      }
      return;
    }

    res.writeHead(400, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: "Bad request" }));
  });

  const shutdown = (): Promise<void> => {
    rateLimiter.destroy();
    for (const [id, session] of sessions) {
      session.transport.close?.();
      sessions.delete(id);
    }
    return new Promise((resolve, reject) => {
      httpServer.close((err) => (err ? reject(err) : resolve()));
    });
  };

  return { httpServer, shutdown };
}

async function main() {
  const config = loadConfig();
  logger.setLevel(config.logLevel);

  const { httpServer, shutdown } = createMcpHttpServer(config);

  httpServer.listen(config.mcpPort, () => {
    logger.info(`GOSO MCP server (HTTP) listening on port ${config.mcpPort}`);
    logger.info(`MCP endpoint: http://localhost:${config.mcpPort}/mcp`);
    logger.info(`Health check: http://localhost:${config.mcpPort}/health`);
  });

  // Graceful shutdown
  const onSignal = () => {
    logger.info("Shutting down...");
    void shutdown().then(
      () => process.exit(0),
      () => process.exit(1),
    );
    // Force exit after 5s
    setTimeout(() => process.exit(1), 5000);
  };

  process.on("SIGINT", onSignal);
  process.on("SIGTERM", onSignal);
}

function isDirectExecution(): boolean {
  const entry = process.argv[1];
  if (!entry) return false;
  try {
    return import.meta.url === pathToFileURL(entry).href;
  } catch {
    return false;
  }
}

if (isDirectExecution()) {
  main().catch((err) => {
    logger.error("Fatal error", { error: String(err) });
    process.exit(1);
  });
}

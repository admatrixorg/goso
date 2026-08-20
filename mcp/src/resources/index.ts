/**
 * Resource registration — read-only data sources for LLM context.
 */

import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";
import { logger } from "../lib/logger.js";

export function registerAllResources(server: McpServer, client: GoClawClient): void {
  // Gateway status
  server.resource("goclaw://status", "Gateway status summary", async () => {
    try {
      const s = await client.system.status();
      return {
        contents: [
          {
            uri: "goclaw://status",
            mimeType: "text/plain",
            text: `GoClaw Gateway v${s.version}\nUptime: ${s.uptime}\nConnections: ${s.connections}\nAgents: ${s.agents}\nSessions: ${s.sessions}`,
          },
        ],
      };
    } catch {
      return { contents: [{ uri: "goclaw://status", mimeType: "text/plain", text: "Unable to fetch status." }] };
    }
  });

  // Available models
  server.resource("goclaw://models", "Available LLM models", async () => {
    try {
      const models = await client.system.listModels();
      const text = models
        .map((m) => `${m.name} (${m.provider}) — ${m.context_window.toLocaleString()} tokens`)
        .join("\n");
      return {
        contents: [
          { uri: "goclaw://models", mimeType: "text/plain", text: text || "No models available." },
        ],
      };
    } catch {
      return { contents: [{ uri: "goclaw://models", mimeType: "text/plain", text: "Unable to fetch models." }] };
    }
  });

  // Agent list
  server.resource("goclaw://agents", "All agents summary", async () => {
    try {
      const agents = await client.agents.listAgents();
      const text = agents
        .map((a) => `${a.display_name} (${a.agent_key}) — ${a.provider}/${a.model} [${a.agent_type}]`)
        .join("\n");
      return {
        contents: [
          { uri: "goclaw://agents", mimeType: "text/plain", text: text || "No agents configured." },
        ],
      };
    } catch {
      return { contents: [{ uri: "goclaw://agents", mimeType: "text/plain", text: "Unable to fetch agents." }] };
    }
  });

  // Current config
  server.resource("goclaw://config", "Current gateway configuration", async () => {
    try {
      const config = await client.config.getConfig();
      return {
        contents: [
          {
            uri: "goclaw://config",
            mimeType: "application/json",
            text: JSON.stringify(config, null, 2),
          },
        ],
      };
    } catch {
      return {
        contents: [{ uri: "goclaw://config", mimeType: "text/plain", text: "Unable to fetch config." }],
      };
    }
  });

  logger.info("All MCP resources registered");
}

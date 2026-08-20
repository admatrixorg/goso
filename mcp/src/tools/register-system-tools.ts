/** System tools: health, status, models list */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerSystemTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_health",
    "Check GoClaw gateway health status",
    {},
    async () => {
      try {
        const health = await client.system.health();
        return {
          content: [
            {
              type: "text",
              text: `Gateway Health: ${health.status}\nVersion: ${health.version}\nUptime: ${health.uptime}`,
            },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_status",
    "Get GoClaw gateway status including version, uptime, and connection counts",
    {},
    async () => {
      try {
        const status = await client.system.status();
        return {
          content: [
            {
              type: "text",
              text: [
                `**GoClaw Gateway Status**`,
                `- Version: ${status.version}`,
                `- Uptime: ${status.uptime}`,
                `- Active connections: ${status.connections}`,
                `- Agents: ${status.agents}`,
                `- Sessions: ${status.sessions}`,
              ].join("\n"),
            },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_models_list",
    "List all available LLM models across all configured providers",
    {},
    async () => {
      try {
        const models = await client.system.listModels();
        if (!models.length) {
          return { content: [{ type: "text", text: "No models available." }] };
        }
        const text = models
          .map((m) => `- **${m.name}** (${m.provider}) — context: ${m.context_window.toLocaleString()} tokens`)
          .join("\n");
        return { content: [{ type: "text", text: `**Available Models**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

/** Config tools: get, apply, patch */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerConfigTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_config_get",
    "Get current GoClaw gateway configuration (or a specific section)",
    {
      section: z.string().optional().describe("Config section (e.g. gateway, agents, tools, channels)"),
    },
    async ({ section }) => {
      try {
        const config = await client.config.getConfig(section);
        return {
          content: [
            {
              type: "text",
              text: `**Configuration${section ? ` (${section})` : ""}**\n\`\`\`json\n${JSON.stringify(config, null, 2)}\n\`\`\``,
            },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_config_apply",
    "Apply a full configuration to the GoClaw gateway (overwrites current config). Use with caution.",
    {
      config: z
        .record(z.string(), z.unknown())
        .describe("Full configuration object to apply"),
    },
    async ({ config }) => {
      try {
        await client.config.applyConfig(config as Record<string, unknown>);
        return { content: [{ type: "text", text: "Configuration applied successfully." }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_config_patch",
    "Patch specific fields in the GoClaw gateway configuration",
    {
      patches: z
        .record(z.string(), z.unknown())
        .describe("Configuration fields to patch (merged into current config)"),
    },
    async ({ patches }) => {
      try {
        await client.config.patchConfig(patches as Record<string, unknown>);
        return { content: [{ type: "text", text: "Configuration patched successfully." }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

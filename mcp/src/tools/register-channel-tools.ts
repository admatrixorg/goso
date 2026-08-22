/** Channel tools: list, toggle */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerChannelTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_channel_list",
    "List all messaging channels (Telegram, Discord, etc.)",
    {},
    async () => {
      try {
        const channels = await client.channels.listChannels();
        if (!channels.length) return { content: [{ type: "text", text: "No channels configured." }] };
        const text = channels
          .map(
            (c) =>
              `- **${c.name}** (${c.type}) [${c.enabled ? "enabled" : "disabled"}] ${c.connected ? "connected" : "disconnected"}`
          )
          .join("\n");
        return { content: [{ type: "text", text: `**Channels**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_channel_toggle",
    "Enable or disable a messaging channel",
    {
      channel: z.string().describe("Channel name"),
      enabled: z.boolean().describe("true to enable, false to disable"),
    },
    async ({ channel, enabled }) => {
      try {
        await client.channels.toggleChannel(channel, enabled);
        return {
          content: [
            { type: "text", text: `Channel "${channel}" ${enabled ? "enabled" : "disabled"}.` },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

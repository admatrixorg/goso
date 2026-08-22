/** Session tools: list, preview, delete, reset, label */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerSessionTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_session_list",
    "List chat sessions, optionally filtered by agent",
    {
      agent_id: z.string().optional().describe("Filter by agent ID"),
      limit: z.number().optional().describe("Max results (default: 20)"),
    },
    async ({ agent_id, limit }) => {
      try {
        const sessions = await client.sessions.listSessions({ agent_id, limit });
        if (!sessions.length) return { content: [{ type: "text", text: "No sessions found." }] };
        const text = sessions
          .map(
            (s) =>
              `- \`${s.session_key}\` — ${s.label || "(no label)"} | msgs: ${s.message_count} | tokens: ${s.input_tokens + s.output_tokens} | ${s.updated_at}`
          )
          .join("\n");
        return { content: [{ type: "text", text: `**Sessions**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_session_preview",
    "Preview recent messages in a chat session",
    {
      session_key: z.string().describe("Session key"),
      limit: z.number().optional().describe("Max messages to show (default: 10)"),
    },
    async ({ session_key, limit }) => {
      try {
        const preview = await client.sessions.previewSession(session_key, limit);
        if (!preview.messages.length) {
          return { content: [{ type: "text", text: "Session is empty." }] };
        }
        const text = preview.messages
          .map((m) => `**${m.role}** (${m.timestamp}):\n${m.content}`)
          .join("\n\n---\n\n");
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_session_delete",
    "Delete a chat session permanently",
    { session_key: z.string().describe("Session key to delete") },
    async ({ session_key }) => {
      try {
        await client.sessions.deleteSession(session_key);
        return { content: [{ type: "text", text: `Session \`${session_key}\` deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_session_reset",
    "Reset a chat session (clear message history)",
    { session_key: z.string().describe("Session key to reset") },
    async ({ session_key }) => {
      try {
        await client.sessions.resetSession(session_key);
        return { content: [{ type: "text", text: `Session \`${session_key}\` reset.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_session_label",
    "Set a label/title for a chat session",
    {
      session_key: z.string().describe("Session key"),
      label: z.string().describe("Label text"),
    },
    async ({ session_key, label }) => {
      try {
        await client.sessions.labelSession(session_key, label);
        return {
          content: [{ type: "text", text: `Session \`${session_key}\` labeled: "${label}"` }],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

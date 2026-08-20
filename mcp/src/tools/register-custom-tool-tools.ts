/** Custom tool tools: CRUD + invoke */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerCustomToolTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_custom_tool_list",
    "List custom tools defined in GoClaw",
    { agent_id: z.string().optional().describe("Filter by agent ID (omit for global tools)") },
    async ({ agent_id }) => {
      try {
        const tools = await client.customTools.listCustomTools(agent_id);
        if (!tools.length) return { content: [{ type: "text", text: "No custom tools found." }] };
        const text = tools
          .map((t) => `- **${t.name}** — ${t.description} [timeout: ${t.timeout_seconds}s]${t.agent_id ? ` (agent: ${t.agent_id})` : " (global)"}`)
          .join("\n");
        return { content: [{ type: "text", text: `**Custom Tools**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_custom_tool_get",
    "Get details of a custom tool",
    { id: z.string().describe("Custom tool ID") },
    async ({ id }) => {
      try {
        const t = await client.customTools.getCustomTool(id);
        const text = [
          `**${t.name}**`,
          `- ID: ${t.id}`,
          `- Description: ${t.description}`,
          `- Command: \`${t.command}\``,
          `- Timeout: ${t.timeout_seconds}s`,
          `- Scope: ${t.agent_id ? `agent ${t.agent_id}` : "global"}`,
          `- Parameters:\n\`\`\`json\n${JSON.stringify(t.parameters, null, 2)}\n\`\`\``,
        ].join("\n");
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_custom_tool_create",
    "Create a new custom tool in GoClaw",
    {
      name: z.string().describe("Tool name"),
      description: z.string().describe("Tool description"),
      parameters: z.record(z.string(), z.unknown()).describe("JSON Schema for tool parameters") as z.ZodType<Record<string, unknown>>,
      command: z.string().describe("Shell command template (use {{.arg}} for substitution)"),
      timeout_seconds: z.number().optional().describe("Execution timeout (default: 30)"),
      agent_id: z.string().optional().describe("Agent ID (omit for global tool)"),
    },
    async (params) => {
      try {
        const t = await client.customTools.createCustomTool(params);
        return { content: [{ type: "text", text: `Custom tool created: **${t.name}** (${t.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_custom_tool_update",
    "Update a custom tool's definition",
    {
      id: z.string().describe("Custom tool ID"),
      name: z.string().optional(),
      description: z.string().optional(),
      command: z.string().optional(),
      timeout_seconds: z.number().optional(),
    },
    async ({ id, ...updates }) => {
      try {
        const t = await client.customTools.updateCustomTool(id, updates);
        return { content: [{ type: "text", text: `Custom tool updated: **${t.name}** (${t.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_custom_tool_delete",
    "Delete a custom tool from GoClaw",
    { id: z.string().describe("Custom tool ID") },
    async ({ id }) => {
      try {
        await client.customTools.deleteCustomTool(id);
        return { content: [{ type: "text", text: `Custom tool ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_custom_tool_invoke",
    "Invoke a custom tool directly with arguments",
    {
      id: z.string().describe("Custom tool ID"),
      args: z.record(z.string(), z.unknown()).describe("Tool arguments matching its parameter schema") as z.ZodType<Record<string, unknown>>,
    },
    async ({ id, args }) => {
      try {
        const result = await client.customTools.invokeCustomTool(id, args);
        return { content: [{ type: "text", text: result.result }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

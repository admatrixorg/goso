/** MCP Server management tools: CRUD + grants */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerMcpServerTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_mcp_server_list",
    "List all registered MCP servers in GoClaw",
    {},
    async () => {
      try {
        const servers = await client.mcpServers.listMcpServers();
        if (!servers.length) return { content: [{ type: "text", text: "No MCP servers registered." }] };
        const text = servers
          .map((s) => `- **${s.name}** (${s.transport}) [${s.enabled ? "enabled" : "disabled"}]`)
          .join("\n");
        return { content: [{ type: "text", text: `**MCP Servers**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_mcp_server_get",
    "Get details of a registered MCP server",
    { id: z.string().describe("MCP server ID") },
    async ({ id }) => {
      try {
        const s = await client.mcpServers.getMcpServer(id);
        const text = [
          `**${s.name}**`,
          `- ID: ${s.id}`,
          `- Transport: ${s.transport}`,
          s.command ? `- Command: ${s.command} ${s.args.join(" ")}` : `- URL: ${s.url}`,
          `- Enabled: ${s.enabled}`,
        ].join("\n");
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_mcp_server_create",
    "Register a new MCP server in GoClaw",
    {
      name: z.string().describe("Server name"),
      transport: z.enum(["stdio", "sse", "streamable-http"]).describe("Transport type"),
      command: z.string().optional().describe("Command to run (stdio transport)"),
      args: z.array(z.string()).optional().describe("Command arguments (stdio transport)"),
      url: z.string().optional().describe("Server URL (HTTP transports)"),
      headers: z.record(z.string(), z.string()).optional().describe("HTTP headers") as z.ZodOptional<z.ZodType<Record<string, string>>>,
      api_key: z.string().optional().describe("API key for authentication"),
      env: z.record(z.string(), z.string()).optional().describe("Environment variables") as z.ZodOptional<z.ZodType<Record<string, string>>>,
      enabled: z.boolean().optional().describe("Enable on creation (default: true)"),
    },
    async (params) => {
      try {
        const s = await client.mcpServers.createMcpServer(params as Parameters<typeof client.mcpServers.createMcpServer>[0]);
        return { content: [{ type: "text", text: `MCP server registered: **${s.name}** (${s.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_mcp_server_update",
    "Update a registered MCP server's configuration",
    {
      id: z.string().describe("MCP server ID"),
      name: z.string().optional().describe("New name"),
      transport: z.enum(["stdio", "sse", "streamable-http"]).optional(),
      command: z.string().optional(),
      args: z.array(z.string()).optional(),
      url: z.string().optional(),
      enabled: z.boolean().optional(),
    },
    async ({ id, ...updates }) => {
      try {
        const s = await client.mcpServers.updateMcpServer(id, updates);
        return { content: [{ type: "text", text: `MCP server updated: **${s.name}** (${s.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_mcp_server_delete",
    "Remove a registered MCP server from GoClaw",
    { id: z.string().describe("MCP server ID") },
    async ({ id }) => {
      try {
        await client.mcpServers.deleteMcpServer(id);
        return { content: [{ type: "text", text: `MCP server ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_mcp_server_grant_agent",
    "Grant an agent access to an MCP server",
    {
      server_id: z.string().describe("MCP server ID"),
      agent_id: z.string().describe("Agent ID to grant access"),
    },
    async ({ server_id, agent_id }) => {
      try {
        await client.mcpServers.grantMcpServerAgent(server_id, agent_id);
        return { content: [{ type: "text", text: `Agent ${agent_id} granted access to MCP server ${server_id}.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_mcp_server_grant_user",
    "Grant a user access to an MCP server",
    {
      server_id: z.string().describe("MCP server ID"),
      user_id: z.string().describe("User ID to grant access"),
    },
    async ({ server_id, user_id }) => {
      try {
        await client.mcpServers.grantMcpServerUser(server_id, user_id);
        return { content: [{ type: "text", text: `User ${user_id} granted access to MCP server ${server_id}.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

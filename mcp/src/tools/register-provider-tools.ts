/** Provider tools: CRUD for LLM providers */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerProviderTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_provider_list",
    "List all configured LLM providers",
    {},
    async () => {
      try {
        const providers = await client.providers.listProviders();
        if (!providers.length) return { content: [{ type: "text", text: "No providers configured." }] };
        const text = providers
          .map((p) => `- **${p.name}** (${p.type}) — ${p.models.length} models [${p.enabled ? "enabled" : "disabled"}]`)
          .join("\n");
        return { content: [{ type: "text", text: `**Providers**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_provider_get",
    "Get details of a specific LLM provider",
    { id: z.string().describe("Provider ID") },
    async ({ id }) => {
      try {
        const p = await client.providers.getProvider(id);
        const text = [
          `**${p.name}** (${p.type})`,
          `- ID: ${p.id}`,
          `- Base URL: ${p.base_url}`,
          `- Models: ${p.models.join(", ")}`,
          `- Enabled: ${p.enabled}`,
        ].join("\n");
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_provider_create",
    "Add a new LLM provider to GoClaw",
    {
      name: z.string().describe("Provider name"),
      type: z.string().describe("Provider type (e.g. openai, anthropic, openrouter)"),
      api_key: z.string().describe("API key for the provider"),
      base_url: z.string().optional().describe("Custom API base URL"),
      models: z.array(z.string()).optional().describe("List of model IDs to enable"),
    },
    async (params) => {
      try {
        const p = await client.providers.createProvider(params);
        return { content: [{ type: "text", text: `Provider created: **${p.name}** (${p.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_provider_update",
    "Update an LLM provider's configuration",
    {
      id: z.string().describe("Provider ID"),
      name: z.string().optional().describe("New name"),
      api_key: z.string().optional().describe("New API key"),
      base_url: z.string().optional().describe("New base URL"),
      models: z.array(z.string()).optional().describe("Updated model list"),
      enabled: z.boolean().optional().describe("Enable/disable provider"),
    },
    async ({ id, ...updates }) => {
      try {
        const p = await client.providers.updateProvider(id, updates);
        return { content: [{ type: "text", text: `Provider updated: **${p.name}** (${p.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_provider_delete",
    "Remove an LLM provider from GoClaw",
    { id: z.string().describe("Provider ID") },
    async ({ id }) => {
      try {
        await client.providers.deleteProvider(id);
        return { content: [{ type: "text", text: `Provider ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

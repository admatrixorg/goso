/** Agent tools: CRUD, context files, links, shares */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerAgentTools(server: McpServer, client: GoClawClient): void {
  // --- CRUD ---

  server.tool(
    "goclaw_agent_list",
    "List all agents configured in GoClaw gateway",
    { include_deleted: z.boolean().optional().describe("Include soft-deleted agents") },
    async ({ include_deleted }) => {
      try {
        const agents = await client.agents.listAgents({ include_deleted });
        if (!agents.length) return { content: [{ type: "text", text: "No agents found." }] };
        const text = agents
          .map(
            (a) =>
              `- **${a.display_name}** (\`${a.agent_key}\`) — ${a.provider}/${a.model} [${a.agent_type}]${a.deleted_at ? " [DELETED]" : ""}`
          )
          .join("\n");
        return { content: [{ type: "text", text: `**Agents**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_get",
    "Get detailed information about a specific agent",
    { id: z.string().describe("Agent ID or agent_key") },
    async ({ id }) => {
      try {
        const a = await client.agents.getAgent(id);
        const text = [
          `**${a.display_name}** (\`${a.agent_key}\`)`,
          `- ID: ${a.id}`,
          `- Provider: ${a.provider}`,
          `- Model: ${a.model}`,
          `- Context window: ${a.context_window.toLocaleString()}`,
          `- Max tool iterations: ${a.max_tool_iterations}`,
          `- Workspace: ${a.workspace}`,
          `- Type: ${a.agent_type}`,
          `- Created: ${a.created_at}`,
        ].join("\n");
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_create",
    "Create a new agent in GoClaw gateway",
    {
      agent_key: z.string().describe("Unique agent identifier (lowercase, no spaces)"),
      display_name: z.string().describe("Human-readable agent name"),
      provider: z.string().optional().describe("LLM provider (e.g. anthropic, openai)"),
      model: z.string().optional().describe("LLM model ID"),
      context_window: z.number().optional().describe("Context window size in tokens"),
      max_tool_iterations: z.number().optional().describe("Max tool call iterations per turn"),
      workspace: z.string().optional().describe("Agent workspace directory"),
      agent_type: z.enum(["open", "predefined"]).optional().describe("Agent type"),
    },
    async (params) => {
      try {
        const agent = await client.agents.createAgent(params);
        return {
          content: [
            {
              type: "text",
              text: `Agent created: **${agent.display_name}** (\`${agent.agent_key}\`)\nID: ${agent.id}`,
            },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_update",
    "Update an existing agent's settings",
    {
      id: z.string().describe("Agent ID"),
      display_name: z.string().optional().describe("New display name"),
      provider: z.string().optional().describe("New LLM provider"),
      model: z.string().optional().describe("New LLM model"),
      context_window: z.number().optional().describe("New context window size"),
      max_tool_iterations: z.number().optional().describe("New max tool iterations"),
      workspace: z.string().optional().describe("New workspace directory"),
    },
    async ({ id, ...updates }) => {
      try {
        const agent = await client.agents.updateAgent(id, updates);
        return {
          content: [
            { type: "text", text: `Agent updated: **${agent.display_name}** (${agent.id})` },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_delete",
    "Delete an agent (soft delete)",
    { id: z.string().describe("Agent ID") },
    async ({ id }) => {
      try {
        await client.agents.deleteAgent(id);
        return { content: [{ type: "text", text: `Agent ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  // --- Context Files ---

  server.tool(
    "goclaw_agent_files_list",
    "List context files (SOUL.md, IDENTITY.md, etc.) for an agent",
    { agent_id: z.string().describe("Agent ID") },
    async ({ agent_id }) => {
      try {
        const files = await client.agents.listAgentFiles(agent_id);
        if (!files.length) return { content: [{ type: "text", text: "No context files." }] };
        const text = files.map((f) => `- \`${f.path}\` (updated: ${f.updated_at})`).join("\n");
        return { content: [{ type: "text", text: `**Context Files**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_files_get",
    "Read a specific context file for an agent",
    {
      agent_id: z.string().describe("Agent ID"),
      path: z.string().describe("File path (e.g. SOUL.md, IDENTITY.md)"),
    },
    async ({ agent_id, path }) => {
      try {
        const file = await client.agents.getAgentFile(agent_id, path);
        return {
          content: [
            { type: "text", text: `**${file.path}**\n\`\`\`\n${file.content}\n\`\`\`` },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_files_set",
    "Create or update a context file for an agent",
    {
      agent_id: z.string().describe("Agent ID"),
      path: z.string().describe("File path (e.g. SOUL.md, IDENTITY.md)"),
      content: z.string().describe("File content"),
    },
    async ({ agent_id, path, content }) => {
      try {
        await client.agents.setAgentFile(agent_id, path, content);
        return { content: [{ type: "text", text: `File \`${path}\` saved for agent ${agent_id}.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_files_delete",
    "Delete a context file from an agent",
    {
      agent_id: z.string().describe("Agent ID"),
      path: z.string().describe("File path to delete"),
    },
    async ({ agent_id, path }) => {
      try {
        await client.agents.deleteAgentFile(agent_id, path);
        return { content: [{ type: "text", text: `File \`${path}\` deleted from agent ${agent_id}.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  // --- Links ---

  server.tool(
    "goclaw_agent_links_list",
    "List delegation links for an agent (which agents it can delegate to)",
    { agent_id: z.string().describe("Agent ID") },
    async ({ agent_id }) => {
      try {
        const links = await client.agents.listAgentLinks(agent_id);
        if (!links.length) return { content: [{ type: "text", text: "No delegation links." }] };
        const text = links
          .map((l) => `- → **${l.target_agent_key}** (${l.target_agent_id}): ${l.description}`)
          .join("\n");
        return { content: [{ type: "text", text: `**Delegation Links**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_links_set",
    "Create or update a delegation link between agents",
    {
      agent_id: z.string().describe("Source agent ID"),
      target_agent_id: z.string().describe("Target agent ID to delegate to"),
      description: z.string().describe("Description of what is delegated"),
    },
    async ({ agent_id, target_agent_id, description }) => {
      try {
        await client.agents.setAgentLink(agent_id, target_agent_id, description);
        return {
          content: [
            { type: "text", text: `Delegation link set: ${agent_id} → ${target_agent_id}` },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_agent_links_remove",
    "Remove a delegation link between agents",
    {
      agent_id: z.string().describe("Source agent ID"),
      target_agent_id: z.string().describe("Target agent ID to unlink"),
    },
    async ({ agent_id, target_agent_id }) => {
      try {
        await client.agents.removeAgentLink(agent_id, target_agent_id);
        return {
          content: [
            { type: "text", text: `Delegation link removed: ${agent_id} → ${target_agent_id}` },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  // --- Shares ---

  server.tool(
    "goclaw_agent_share",
    "Share an agent with a user",
    {
      agent_id: z.string().describe("Agent ID"),
      user_id: z.string().describe("User ID to share with"),
    },
    async ({ agent_id, user_id }) => {
      try {
        await client.agents.shareAgent(agent_id, { user_id });
        return {
          content: [{ type: "text", text: `Agent ${agent_id} shared with user ${user_id}.` }],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

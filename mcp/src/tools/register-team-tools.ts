/** Team tools: CRUD */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerTeamTools(server: McpServer, client: GoClawClient): void {
  server.tool("goclaw_team_list", "List all teams in GoClaw", {}, async () => {
    try {
      const teams = await client.teams.listTeams();
      if (!teams.length) return { content: [{ type: "text", text: "No teams." }] };
      const text = teams
        .map((t) => `- **${t.name}** — ${t.description} (${t.member_agent_ids.length} agents)`)
        .join("\n");
      return { content: [{ type: "text", text: `**Teams**\n${text}` }] };
    } catch (err) {
      return handleToolError(err);
    }
  });

  server.tool(
    "goclaw_team_get",
    "Get team details",
    { id: z.string().describe("Team ID") },
    async ({ id }) => {
      try {
        const t = await client.teams.getTeam(id);
        return {
          content: [
            {
              type: "text",
              text: `**${t.name}**\n- ID: ${t.id}\n- Description: ${t.description}\n- Members: ${t.member_agent_ids.join(", ")}`,
            },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_team_create",
    "Create a new agent team",
    {
      name: z.string().describe("Team name"),
      description: z.string().optional().describe("Team description"),
      member_agent_ids: z.array(z.string()).describe("Agent IDs to include"),
    },
    async (params) => {
      try {
        const t = await client.teams.createTeam(params);
        return { content: [{ type: "text", text: `Team created: **${t.name}** (${t.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_team_update",
    "Update a team's settings",
    {
      id: z.string().describe("Team ID"),
      name: z.string().optional(),
      description: z.string().optional(),
      member_agent_ids: z.array(z.string()).optional(),
    },
    async ({ id, ...updates }) => {
      try {
        const t = await client.teams.updateTeam(id, updates);
        return { content: [{ type: "text", text: `Team updated: **${t.name}** (${t.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_team_delete",
    "Delete a team",
    { id: z.string().describe("Team ID") },
    async ({ id }) => {
      try {
        await client.teams.deleteTeam(id);
        return { content: [{ type: "text", text: `Team ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

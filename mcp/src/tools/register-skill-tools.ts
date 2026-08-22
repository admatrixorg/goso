/** Skill tools: list, get, update, grants */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerSkillTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_skill_list",
    "List all available skills in GoClaw",
    {},
    async () => {
      try {
        const skills = await client.skills.listSkills();
        if (!skills.length) return { content: [{ type: "text", text: "No skills available." }] };
        const text = skills
          .map((s) => `- **${s.name}** — ${s.description} [${s.enabled ? "enabled" : "disabled"}]`)
          .join("\n");
        return { content: [{ type: "text", text: `**Skills**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_skill_get",
    "Get details of a specific skill",
    { id: z.string().describe("Skill ID") },
    async ({ id }) => {
      try {
        const s = await client.skills.getSkill(id);
        return {
          content: [
            {
              type: "text",
              text: `**${s.name}**\n- ID: ${s.id}\n- Description: ${s.description}\n- Path: ${s.path}\n- Enabled: ${s.enabled}`,
            },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_skill_update",
    "Update a skill's metadata",
    {
      id: z.string().describe("Skill ID"),
      name: z.string().optional().describe("New name"),
      description: z.string().optional().describe("New description"),
      enabled: z.boolean().optional().describe("Enable/disable"),
    },
    async ({ id, ...updates }) => {
      try {
        const s = await client.skills.updateSkill(id, updates);
        return { content: [{ type: "text", text: `Skill updated: **${s.name}** (${s.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_skill_grant_agent",
    "Grant an agent access to a skill",
    {
      skill_id: z.string().describe("Skill ID"),
      agent_id: z.string().describe("Agent ID"),
    },
    async ({ skill_id, agent_id }) => {
      try {
        await client.skills.grantSkillAgent(skill_id, agent_id);
        return { content: [{ type: "text", text: `Agent ${agent_id} granted access to skill ${skill_id}.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_skill_grant_user",
    "Grant a user access to a skill",
    {
      skill_id: z.string().describe("Skill ID"),
      user_id: z.string().describe("User ID"),
    },
    async ({ skill_id, user_id }) => {
      try {
        await client.skills.grantSkillUser(skill_id, user_id);
        return { content: [{ type: "text", text: `User ${user_id} granted access to skill ${skill_id}.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

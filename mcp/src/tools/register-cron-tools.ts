/** Cron tools: CRUD + toggle + run */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerCronTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_cron_list",
    "List all cron jobs in GoClaw",
    {},
    async () => {
      try {
        const jobs = await client.cron.listCronJobs();
        if (!jobs.length) return { content: [{ type: "text", text: "No cron jobs." }] };
        const text = jobs
          .map(
            (j) =>
              `- **${j.name}** (\`${j.expression}\`) → agent ${j.agent_id} [${j.enabled ? "enabled" : "disabled"}]${j.next_run ? ` | next: ${j.next_run}` : ""}`
          )
          .join("\n");
        return { content: [{ type: "text", text: `**Cron Jobs**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_cron_create",
    "Create a new cron job",
    {
      name: z.string().describe("Job name"),
      expression: z.string().describe("Cron expression (e.g. '0 */6 * * *')"),
      agent_id: z.string().describe("Agent ID to run the job with"),
      message: z.string().describe("Message to send to the agent"),
      enabled: z.boolean().optional().describe("Enable on creation (default: true)"),
    },
    async (params) => {
      try {
        const j = await client.cron.createCronJob(params);
        return { content: [{ type: "text", text: `Cron job created: **${j.name}** (${j.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_cron_update",
    "Update a cron job's settings",
    {
      id: z.string().describe("Cron job ID"),
      name: z.string().optional(),
      expression: z.string().optional(),
      agent_id: z.string().optional(),
      message: z.string().optional(),
    },
    async ({ id, ...updates }) => {
      try {
        const j = await client.cron.updateCronJob(id, updates);
        return { content: [{ type: "text", text: `Cron job updated: **${j.name}** (${j.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_cron_delete",
    "Delete a cron job",
    { id: z.string().describe("Cron job ID") },
    async ({ id }) => {
      try {
        await client.cron.deleteCronJob(id);
        return { content: [{ type: "text", text: `Cron job ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_cron_toggle",
    "Enable or disable a cron job",
    {
      id: z.string().describe("Cron job ID"),
      enabled: z.boolean().describe("true to enable, false to disable"),
    },
    async ({ id, enabled }) => {
      try {
        await client.cron.toggleCronJob(id, enabled);
        return {
          content: [{ type: "text", text: `Cron job ${id} ${enabled ? "enabled" : "disabled"}.` }],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_cron_run",
    "Trigger a cron job to run immediately",
    { id: z.string().describe("Cron job ID") },
    async ({ id }) => {
      try {
        await client.cron.runCronJob(id);
        return { content: [{ type: "text", text: `Cron job ${id} triggered.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

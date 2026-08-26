/**
 * Prompt registration — reusable conversation templates for GoClaw workflows.
 */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GosoClient } from "../client/index.js";
import { logger } from "../lib/logger.js";

export function registerAllPrompts(server: McpServer, _client: GosoClient): void {
  server.prompt(
    "goso_setup_agent",
    "Guide through creating and configuring a new GoClaw agent",
    { agent_purpose: z.string().describe("What the agent should do") },
    async ({ agent_purpose }) => ({
      messages: [
        {
          role: "user" as const,
          content: {
            type: "text" as const,
            text: [
              `Help me create a GoClaw agent for: ${agent_purpose}`,
              "",
              "Please:",
              "1. Suggest an agent_key and display_name",
              "2. Recommend provider/model based on the purpose",
              "3. Suggest appropriate tool permissions and workspace settings",
              "4. Draft the IDENTITY.md context file content",
              "5. Create the agent using goso_agent_create",
              "6. Set the context file using goso_agent_files_set",
            ].join("\n"),
          },
        },
      ],
    })
  );

  server.prompt(
    "goso_troubleshoot",
    "Systematic troubleshooting of GoClaw gateway issues",
    { symptom: z.string().describe("What issue you are experiencing") },
    async ({ symptom }) => ({
      messages: [
        {
          role: "user" as const,
          content: {
            type: "text" as const,
            text: [
              `I'm experiencing this issue with my GoClaw gateway: ${symptom}`,
              "",
              "Please help me troubleshoot by:",
              "1. Check gateway health using goso_health",
              "2. Check gateway status using goso_status",
              "3. Review current config using goso_config_get",
              "4. Check recent traces for errors using goso_trace_list",
              "5. Review channel status using goso_channel_list",
              "6. Analyze findings and suggest fixes",
            ].join("\n"),
          },
        },
      ],
    })
  );

  server.prompt(
    "goso_review_config",
    "Review current GoClaw configuration for issues or improvements",
    {},
    async () => ({
      messages: [
        {
          role: "user" as const,
          content: {
            type: "text" as const,
            text: [
              "Review my GoClaw gateway configuration for potential issues or improvements.",
              "",
              "Please:",
              "1. Fetch the current config using goso_config_get",
              "2. List providers using goso_provider_list",
              "3. List agents using goso_agent_list",
              "4. Check for security concerns (open tools, missing restrictions)",
              "5. Check for performance issues (context window sizes, tool iterations)",
              "6. Suggest improvements with specific config patches",
            ].join("\n"),
          },
        },
      ],
    })
  );

  server.prompt(
    "goso_optimize_agent",
    "Suggest optimizations for an agent's settings and behavior",
    { agent_id: z.string().describe("Agent ID to optimize") },
    async ({ agent_id }) => ({
      messages: [
        {
          role: "user" as const,
          content: {
            type: "text" as const,
            text: [
              `Analyze and optimize GoClaw agent ${agent_id}.`,
              "",
              "Please:",
              "1. Get agent details using goso_agent_get",
              "2. Review context files using goso_agent_files_list and goso_agent_files_get",
              "3. Check delegation links using goso_agent_links_list",
              "4. Review recent traces for this agent using goso_trace_list",
              "5. Suggest model/provider changes for better cost/performance",
              "6. Suggest context file improvements",
              "7. Suggest tool permission optimizations",
            ].join("\n"),
          },
        },
      ],
    })
  );

  logger.info("All MCP prompts registered");
}

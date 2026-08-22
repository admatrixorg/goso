/** Memory tools: list, get, create, delete */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerMemoryTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_memory_list",
    "List memory documents stored for an agent",
    {
      agent_id: z.string().describe("Agent ID"),
      user_id: z.string().optional().describe("Filter by user ID"),
    },
    async ({ agent_id, user_id }) => {
      try {
        const docs = await client.memory.listMemoryDocs(agent_id, user_id);
        if (!docs.length) return { content: [{ type: "text", text: "No memory documents." }] };
        const text = docs
          .map((d) => `- \`${d.path}\` (${d.id}) — ${d.created_at}`)
          .join("\n");
        return { content: [{ type: "text", text: `**Memory Documents**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_memory_get",
    "Read a memory document's content",
    { id: z.string().describe("Memory document ID") },
    async ({ id }) => {
      try {
        const d = await client.memory.getMemoryDoc(id);
        return {
          content: [
            { type: "text", text: `**${d.path}**\n\`\`\`\n${d.content}\n\`\`\`` },
          ],
        };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_memory_create",
    "Store a new memory document for an agent",
    {
      agent_id: z.string().describe("Agent ID"),
      path: z.string().describe("Document path/name"),
      content: z.string().describe("Document content"),
    },
    async ({ agent_id, path, content }) => {
      try {
        const d = await client.memory.createMemoryDoc(agent_id, path, content);
        return { content: [{ type: "text", text: `Memory document created: \`${d.path}\` (${d.id})` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_memory_delete",
    "Delete a memory document",
    { id: z.string().describe("Memory document ID") },
    async ({ id }) => {
      try {
        await client.memory.deleteMemoryDoc(id);
        return { content: [{ type: "text", text: `Memory document ${id} deleted.` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

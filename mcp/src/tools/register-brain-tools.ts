import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GosoClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerBrainTools(server: McpServer, client: GosoClient): void {
  const tenantField = { tenant: z.string().describe("Goso tenant id") };
  server.tool("search_wiki", "Search the tenant company wiki (Arkon).", { ...tenantField, query: z.string() }, async ({ tenant, query }) => {
    try {
      const res = await client.brain.search(tenant, query);
      return { content: [{ type: "text", text: JSON.stringify(res, null, 2) }] };
    } catch (err) { return handleToolError(err); }
  });
  server.tool("read_wiki_page", "Read one compiled wiki page.", { ...tenantField, id: z.string() }, async ({ tenant, id }) => {
    try {
      const res = await client.brain.getPage(tenant, id);
      return { content: [{ type: "text", text: JSON.stringify(res, null, 2) }] };
    } catch (err) { return handleToolError(err); }
  });
  server.tool("list_wiki_pages", "List tenant wiki pages.", tenantField, async ({ tenant }) => {
    try {
      const res = await client.brain.listPages(tenant);
      return { content: [{ type: "text", text: JSON.stringify(res, null, 2) }] };
    } catch (err) { return handleToolError(err); }
  });
  server.tool("read_wiki_index", "Short index of the tenant wiki.", tenantField, async ({ tenant }) => {
    try {
      const res = await client.brain.listPages(tenant);
      return { content: [{ type: "text", text: JSON.stringify(res, null, 2) }] };
    } catch (err) { return handleToolError(err); }
  });
  server.tool("list_sources", "List source documents compiled into the wiki.", tenantField, async ({ tenant }) => {
    try {
      const res = await client.brain.listSources(tenant);
      return { content: [{ type: "text", text: JSON.stringify(res, null, 2) }] };
    } catch (err) { return handleToolError(err); }
  });
  server.tool("goso_brain_search", "Search tenant company brain.", { ...tenantField, query: z.string() }, async ({ tenant, query }) => {
    try {
      const res = await client.brain.search(tenant, query);
      return { content: [{ type: "text", text: JSON.stringify(res, null, 2) }] };
    } catch (err) { return handleToolError(err); }
  });
}

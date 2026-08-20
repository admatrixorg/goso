/**
 * MCP server factory — creates a configured McpServer instance
 * with all tools, resources, and prompts registered.
 */

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { type Config } from "./config.js";
import { GoClawClient } from "./client/index.js";
import { registerAllTools } from "./tools/index.js";
import { registerAllResources } from "./resources/index.js";
import { registerAllPrompts } from "./prompts/index.js";
import { logger } from "./lib/logger.js";
import { SERVER_NAME, SERVER_VERSION } from "./version.js";

/**
 * Create a fully configured MCP server with GOSO tools, resources, and prompts.
 * Tool names remain goclaw_* (66 tools); aliases land in a later spec.
 */
export function createServer(config: Config): McpServer {
  const server = new McpServer({
    name: SERVER_NAME,
    version: SERVER_VERSION,
  });

  const client = new GoClawClient({
    baseUrl: config.goClawServer,
    token: config.goClawToken,
    userId: config.goClawUserId,
  });

  logger.info("Registering MCP capabilities", {
    server: config.goClawServer,
    hasToken: !!config.goClawToken,
    userId: config.goClawUserId ?? "(default)",
  });

  registerAllTools(server, client);
  registerAllResources(server, client);
  registerAllPrompts(server, client);

  return server;
}

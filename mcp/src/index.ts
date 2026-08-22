/**
 * GOSO MCP Server — stdio transport entry point.
 * Used for local development and direct integration with Claude Code, Cursor, etc.
 */

import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { loadConfig } from "./config.js";
import { createServer } from "./server.js";
import { logger } from "./lib/logger.js";

async function main() {
  const config = loadConfig();
  logger.setLevel(config.logLevel);

  const server = createServer(config);
  const transport = new StdioServerTransport();

  logger.info("Starting GOSO MCP server (stdio transport)");
  await server.connect(transport);
}

main().catch((err) => {
  logger.error("Fatal error", { error: String(err) });
  process.exit(1);
});

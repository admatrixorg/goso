/** Trace tools: list, detail */

import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { GoClawClient } from "../client/index.js";
import { handleToolError } from "../lib/errors.js";

export function registerTraceTools(server: McpServer, client: GoClawClient): void {
  server.tool(
    "goclaw_trace_list",
    "List LLM execution traces with cost and token usage",
    {
      agent_id: z.string().optional().describe("Filter by agent ID"),
      limit: z.number().optional().describe("Max results (default: 20)"),
      status: z.string().optional().describe("Filter by status (success, error)"),
    },
    async (params) => {
      try {
        const traces = await client.traces.listTraces(params);
        if (!traces.length) return { content: [{ type: "text", text: "No traces found." }] };
        const text = traces
          .map(
            (t) =>
              `- \`${t.trace_id}\` [${t.status}] — ${t.duration_ms}ms | in: ${t.input_tokens} out: ${t.output_tokens} | cost: $${t.cost} | ${t.created_at}`
          )
          .join("\n");
        return { content: [{ type: "text", text: `**Traces**\n${text}` }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );

  server.tool(
    "goclaw_trace_get",
    "Get detailed trace with individual LLM call spans",
    { trace_id: z.string().describe("Trace ID") },
    async ({ trace_id }) => {
      try {
        const t = await client.traces.getTrace(trace_id);
        const spans = t.spans
          .map(
            (s) =>
              `  - ${s.name} (${s.provider}/${s.model}) [${s.status}] — ${s.duration_ms}ms | in: ${s.input_tokens} out: ${s.output_tokens}`
          )
          .join("\n");
        const text = [
          `**Trace ${t.trace_id}** [${t.status}]`,
          `- Duration: ${t.duration_ms}ms`,
          `- Tokens: in=${t.input_tokens} out=${t.output_tokens}`,
          `- Cost: $${t.cost}`,
          `- Spans:\n${spans}`,
        ].join("\n");
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return handleToolError(err);
      }
    }
  );
}

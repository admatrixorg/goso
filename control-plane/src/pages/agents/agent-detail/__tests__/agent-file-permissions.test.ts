import { describe, expect, it } from "vitest";
import { canManageAgentFiles } from "../agent-file-permissions";

describe("canManageAgentFiles", () => {
  const mcpAgent = {
    agent_type: "predefined" as const,
    owner_id: "goclaw-mcp",
  };

  it("allows the system owner to edit an MCP-created agent", () => {
    expect(canManageAgentFiles(mcpAgent, "mqglobal", true)).toBe(true);
  });

  it("allows the agent owner to edit its own predefined agent", () => {
    expect(canManageAgentFiles(mcpAgent, "goclaw-mcp", false)).toBe(true);
  });

  it("keeps unrelated non-owner users read-only", () => {
    expect(canManageAgentFiles(mcpAgent, "other-user", false)).toBe(false);
  });

  it("preserves editing for open agents", () => {
    expect(
      canManageAgentFiles(
        { agent_type: "open", owner_id: "other-user" },
        "mqglobal",
        false,
      ),
    ).toBe(true);
  });
});

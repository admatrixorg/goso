import { describe, it, expect } from "vitest";
import { GoClawError, handleToolError } from "../../src/lib/errors.js";

describe("GoClawError", () => {
  it("creates error with code and statusCode", () => {
    const err = new GoClawError({
      code: "ERR_NOT_FOUND",
      message: "Agent not found",
      status_code: 404,
    });
    expect(err.message).toBe("Agent not found");
    expect(err.code).toBe("ERR_NOT_FOUND");
    expect(err.statusCode).toBe(404);
    expect(err.name).toBe("GoClawError");
  });

  it("formats as MCP error content", () => {
    const err = new GoClawError({
      code: "ERR_FORBIDDEN",
      message: "Access denied",
      status_code: 403,
    });
    const content = err.toMcpContent();
    expect(content.isError).toBe(true);
    expect(content.content[0].type).toBe("text");
    expect(content.content[0].text).toContain("ERR_FORBIDDEN");
    expect(content.content[0].text).toContain("Access denied");
  });
});

describe("handleToolError", () => {
  it("handles GoClawError", () => {
    const err = new GoClawError({
      code: "ERR_NOT_FOUND",
      message: "Not found",
      status_code: 404,
    });
    const result = handleToolError(err);
    expect(result.isError).toBe(true);
    expect(result.content[0].text).toContain("ERR_NOT_FOUND");
  });

  it("handles generic Error", () => {
    const result = handleToolError(new Error("Something broke"));
    expect(result.isError).toBe(true);
    expect(result.content[0].text).toContain("Something broke");
  });

  it("handles string error", () => {
    const result = handleToolError("string error");
    expect(result.isError).toBe(true);
    expect(result.content[0].text).toContain("string error");
  });
});

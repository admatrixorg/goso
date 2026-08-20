/**
 * Error types for GoClaw API responses.
 * Maps GoClaw API errors to structured MCP error content.
 */

/** GoClaw API error structure */
export interface ApiErrorData {
  code: string;
  message: string;
  status_code: number;
}

/** Error thrown when GoClaw API returns ok: false */
export class GoClawError extends Error {
  public readonly code: string;
  public readonly statusCode: number;

  constructor(data: ApiErrorData) {
    super(data.message);
    this.name = "GoClawError";
    this.code = data.code;
    this.statusCode = data.status_code;
  }

  /** Format as MCP error content */
  toMcpContent() {
    return {
      content: [
        {
          type: "text" as const,
          text: `Error [${this.code}]: ${this.message}`,
        },
      ],
      isError: true,
    };
  }
}

/** Wrap tool handler errors into MCP error content */
export function handleToolError(err: unknown) {
  if (err instanceof GoClawError) {
    return err.toMcpContent();
  }

  const message = err instanceof Error ? err.message : String(err);
  return {
    content: [{ type: "text" as const, text: `Error: ${message}` }],
    isError: true,
  };
}

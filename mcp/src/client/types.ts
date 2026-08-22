/**
 * GoClaw API request/response types.
 * Mirrors the GoClaw Gateway REST API structures.
 */

// === API Envelope ===

export interface ApiResponse<T> {
  ok: boolean;
  payload: T;
  error?: ApiErrorResponse;
}

export interface ApiErrorResponse {
  code: string;
  message: string;
  status_code: number;
}

// === Agent ===

export interface Agent {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  context_window: number;
  max_tool_iterations: number;
  workspace: string;
  restrict_to_workspace: boolean;
  agent_type: "open" | "predefined";
  tools_config: Record<string, unknown>;
  sandbox_config: Record<string, unknown>;
  memory_config: Record<string, unknown>;
  status: string;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
}

export interface CreateAgentRequest {
  agent_key: string;
  display_name: string;
  provider?: string;
  model?: string;
  context_window?: number;
  max_tool_iterations?: number;
  workspace?: string;
  restrict_to_workspace?: boolean;
  agent_type?: "open" | "predefined";
  tools_config?: Record<string, unknown>;
}

export interface UpdateAgentRequest {
  display_name?: string;
  provider?: string;
  model?: string;
  context_window?: number;
  max_tool_iterations?: number;
  workspace?: string;
  restrict_to_workspace?: boolean;
  tools_config?: Record<string, unknown>;
  sandbox_config?: Record<string, unknown>;
  memory_config?: Record<string, unknown>;
}

// === Agent Context Files ===

export interface AgentFile {
  path: string;
  content: string;
  updated_at: string;
}

// === Agent Links ===

export interface AgentLink {
  target_agent_id: string;
  target_agent_key: string;
  description: string;
}

// === Session ===

export interface Session {
  session_key: string;
  agent_id: string;
  user_id: string;
  label: string;
  input_tokens: number;
  output_tokens: number;
  message_count: number;
  created_at: string;
  updated_at: string;
}

export interface SessionPreview {
  session_key: string;
  messages: SessionMessage[];
}

export interface SessionMessage {
  role: "user" | "assistant" | "system";
  content: string;
  timestamp: string;
}

// === Config ===

export interface GatewayConfig {
  [key: string]: unknown;
}

// === Provider ===

export interface Provider {
  id: string;
  name: string;
  type: string;
  base_url: string;
  models: string[];
  enabled: boolean;
  created_at: string;
}

export interface CreateProviderRequest {
  name: string;
  type: string;
  api_key: string;
  base_url?: string;
  models?: string[];
}

export interface UpdateProviderRequest {
  name?: string;
  api_key?: string;
  base_url?: string;
  models?: string[];
  enabled?: boolean;
}

// === MCP Server ===

export interface McpServerEntry {
  id: string;
  name: string;
  transport: "stdio" | "sse" | "streamable-http";
  command: string;
  args: string[];
  url: string;
  headers: Record<string, string>;
  enabled: boolean;
  created_at: string;
}

export interface CreateMcpServerRequest {
  name: string;
  transport: "stdio" | "sse" | "streamable-http";
  command?: string;
  args?: string[];
  url?: string;
  headers?: Record<string, string>;
  api_key?: string;
  env?: Record<string, string>;
  enabled?: boolean;
}

export interface UpdateMcpServerRequest {
  name?: string;
  transport?: "stdio" | "sse" | "streamable-http";
  command?: string;
  args?: string[];
  url?: string;
  headers?: Record<string, string>;
  api_key?: string;
  env?: Record<string, string>;
  enabled?: boolean;
}

// === Skill ===

export interface Skill {
  id: string;
  name: string;
  description: string;
  path: string;
  enabled: boolean;
  created_at: string;
}

// === Custom Tool ===

export interface CustomTool {
  id: string;
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  command: string;
  timeout_seconds: number;
  agent_id: string | null;
  created_at: string;
}

export interface CreateCustomToolRequest {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  command: string;
  timeout_seconds?: number;
  agent_id?: string;
  env?: Record<string, string>;
}

export interface UpdateCustomToolRequest {
  name?: string;
  description?: string;
  parameters?: Record<string, unknown>;
  command?: string;
  timeout_seconds?: number;
  env?: Record<string, string>;
}

// === Cron ===

export interface CronJob {
  id: string;
  name: string;
  expression: string;
  agent_id: string;
  message: string;
  enabled: boolean;
  last_run: string | null;
  next_run: string | null;
  created_at: string;
}

export interface CreateCronRequest {
  name: string;
  expression: string;
  agent_id: string;
  message: string;
  enabled?: boolean;
}

export interface UpdateCronRequest {
  name?: string;
  expression?: string;
  agent_id?: string;
  message?: string;
}

// === Team ===

export interface Team {
  id: string;
  name: string;
  description: string;
  member_agent_ids: string[];
  created_at: string;
}

export interface CreateTeamRequest {
  name: string;
  description?: string;
  member_agent_ids: string[];
}

export interface UpdateTeamRequest {
  name?: string;
  description?: string;
  member_agent_ids?: string[];
}

// === Trace ===

export interface Trace {
  trace_id: string;
  agent_id: string;
  session_key: string;
  status: string;
  duration_ms: number;
  input_tokens: number;
  output_tokens: number;
  cost: string;
  created_at: string;
}

export interface TraceDetail extends Trace {
  spans: TraceSpan[];
}

export interface TraceSpan {
  span_id: string;
  parent_span_id: string | null;
  name: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  duration_ms: number;
  status: string;
}

// === Channel ===

export interface Channel {
  name: string;
  type: string;
  enabled: boolean;
  connected: boolean;
}

// === Memory ===

export interface MemoryDocument {
  id: string;
  agent_id: string;
  user_id: string;
  path: string;
  content: string;
  hash: string;
  created_at: string;
}

// === System ===

export interface HealthStatus {
  status: string;
  version: string;
  uptime: string;
}

export interface GatewayStatus {
  version: string;
  uptime: string;
  connections: number;
  agents: number;
  sessions: number;
}

export interface Model {
  id: string;
  provider: string;
  name: string;
  context_window: number;
}

// === Grant ===

export interface GrantRequest {
  agent_id?: string;
  user_id?: string;
}

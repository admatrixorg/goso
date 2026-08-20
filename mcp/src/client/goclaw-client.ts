/**
 * GoClawClient — aggregates all endpoint modules into a single typed client.
 * Thin wrapper over HttpClient with domain-specific methods.
 */

import { HttpClient, type HttpClientOptions } from "./http-client.js";
import { systemEndpoints } from "./endpoints/system-endpoints.js";
import { agentEndpoints } from "./endpoints/agent-endpoints.js";
import { sessionEndpoints } from "./endpoints/session-endpoints.js";
import { configEndpoints } from "./endpoints/config-endpoints.js";
import { providerEndpoints } from "./endpoints/provider-endpoints.js";
import { mcpServerEndpoints } from "./endpoints/mcp-server-endpoints.js";
import { skillEndpoints } from "./endpoints/skill-endpoints.js";
import { customToolEndpoints } from "./endpoints/custom-tool-endpoints.js";
import { cronEndpoints } from "./endpoints/cron-endpoints.js";
import { teamEndpoints } from "./endpoints/team-endpoints.js";
import { traceEndpoints } from "./endpoints/trace-endpoints.js";
import { channelEndpoints } from "./endpoints/channel-endpoints.js";
import { memoryEndpoints } from "./endpoints/memory-endpoints.js";

export interface GoClawClientOptions {
  baseUrl: string;
  token?: string;
  userId?: string;
}

export class GoClawClient {
  private http: HttpClient;

  // Domain endpoint groups
  public system: ReturnType<typeof systemEndpoints>;
  public agents: ReturnType<typeof agentEndpoints>;
  public sessions: ReturnType<typeof sessionEndpoints>;
  public config: ReturnType<typeof configEndpoints>;
  public providers: ReturnType<typeof providerEndpoints>;
  public mcpServers: ReturnType<typeof mcpServerEndpoints>;
  public skills: ReturnType<typeof skillEndpoints>;
  public customTools: ReturnType<typeof customToolEndpoints>;
  public cron: ReturnType<typeof cronEndpoints>;
  public teams: ReturnType<typeof teamEndpoints>;
  public traces: ReturnType<typeof traceEndpoints>;
  public channels: ReturnType<typeof channelEndpoints>;
  public memory: ReturnType<typeof memoryEndpoints>;

  constructor(options: GoClawClientOptions) {
    this.http = new HttpClient(options);

    this.system = systemEndpoints(this.http);
    this.agents = agentEndpoints(this.http);
    this.sessions = sessionEndpoints(this.http);
    this.config = configEndpoints(this.http);
    this.providers = providerEndpoints(this.http);
    this.mcpServers = mcpServerEndpoints(this.http);
    this.skills = skillEndpoints(this.http);
    this.customTools = customToolEndpoints(this.http);
    this.cron = cronEndpoints(this.http);
    this.teams = teamEndpoints(this.http);
    this.traces = traceEndpoints(this.http);
    this.channels = channelEndpoints(this.http);
    this.memory = memoryEndpoints(this.http);
  }
}

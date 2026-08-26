# goso-mcp

MCP (Model Context Protocol) server for [GOSO](https://github.com/mqglobal/goso) Gateway management. Rebrand of `goclaw-mcp`: same 66 tools, dual transport (`stdio` + Streamable HTTP), env vars `GOSO_*`.

Tool names are `goso_*`.

## Quick Start

### stdio (Claude Code, Cursor, etc.)

```bash
npx goso-mcp
```

**Claude Code** (`~/.claude.json` or project `.mcp.json`):

```json
{
  "mcpServers": {
    "goso": {
      "command": "npx",
      "args": ["goso-mcp"],
      "env": {
        "GOSO_GATEWAY_URL": "http://localhost:8080",
        "GOSO_TOKEN": "your-admin-token"
      }
    }
  }
}
```

Local checkout (this repo):

```json
{
  "mcpServers": {
    "goso": {
      "command": "node",
      "args": ["mcp/dist/index.js"],
      "env": {
        "GOSO_GATEWAY_URL": "http://localhost:8080",
        "GOSO_TOKEN": "your-admin-token"
      }
    }
  }
}
```

**Cursor** (`.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "goso": {
      "command": "npx",
      "args": ["goso-mcp"],
      "env": {
        "GOSO_GATEWAY_URL": "http://localhost:8080",
        "GOSO_TOKEN": "your-admin-token"
      }
    }
  }
}
```

### Streamable HTTP (production, multi-client)

```bash
GOSO_GATEWAY_URL=http://localhost:8080 \
GOSO_TOKEN=your-token \
GOSO_MCP_PORT=3100 \
npx goso-mcp-http
```

MCP endpoint: `http://localhost:3100/mcp`  
Health: `http://localhost:3100/health`

## Configuration

`GOSO_*` is preferred. Legacy `GOCLAW_*` still works as fallback.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GOSO_GATEWAY_URL` | Yes | — | GOSO (or GoClaw-compatible) gateway URL |
| `GOSO_TOKEN` | No | — | Bearer token forwarded to the gateway |
| `GOSO_USER_ID` | No | — | Default user ID for multi-tenant scoping |
| `GOSO_MCP_PORT` | No | `3100` | HTTP transport port |
| `GOSO_MCP_ALLOWED_ORIGINS` | No | `localhost` | Comma-separated allowed origins |
| `GOSO_MCP_RATE_LIMIT_RPM` | No | `60` | Rate limit per session (req/min) |
| `GOSO_LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |

Legacy mapping: `GOCLAW_SERVER` → `GOSO_GATEWAY_URL`, `GOCLAW_TOKEN` → `GOSO_TOKEN`, `GOCLAW_MCP_PORT` → `GOSO_MCP_PORT`.

## Available tools (66)

| Domain | Count | Examples |
|--------|-------|----------|
| System | 3 | `goso_health`, `goso_status`, `goso_models_list` |
| Agents | 13 | `goso_agent_list`, `goso_agent_create`, … |
| Sessions | 5 | `goso_session_list`, `goso_session_preview`, … |
| Configuration | 3 | `goso_config_get`, `goso_config_apply`, `goso_config_patch` |
| Providers | 5 | `goso_provider_list`, `goso_provider_create`, … |
| MCP Servers | 7 | `goso_mcp_server_list`, … |
| Skills | 5 | `goso_skill_list`, … |
| Custom Tools | 6 | `goso_custom_tool_list`, `goso_custom_tool_invoke`, … |
| Cron | 6 | `goso_cron_list`, `goso_cron_run`, … |
| Teams | 5 | `goso_team_list`, … |
| Traces | 2 | `goso_trace_list`, `goso_trace_get` |
| Channels | 2 | `goso_channel_list`, `goso_channel_toggle` |
| Memory | 4 | `goso_memory_list`, `goso_memory_create`, … |

Covers gateway administration: agents, sessions, channels, providers.

## Development

```bash
pnpm -C mcp install
pnpm -C mcp verify   # tsc + tests + build
```

From repo root: `make verify` also runs `pnpm -C mcp verify`.

## License

MIT (fork/rebrand of `goclaw-mcp`)

# goso-mcp

MCP (Model Context Protocol) server for [GOSO](https://github.com/mqglobal/goso) Gateway management. Rebrand of `goclaw-mcp`: same 66 tools, dual transport (`stdio` + Streamable HTTP), env vars `GOSO_*`.

Tool names stay `goclaw_*` in this release (aliases in a later spec).

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

Unchanged names from `goclaw-mcp`:

| Domain | Count | Examples |
|--------|-------|----------|
| System | 3 | `goclaw_health`, `goclaw_status`, `goclaw_models_list` |
| Agents | 13 | `goclaw_agent_list`, `goclaw_agent_create`, … |
| Sessions | 5 | `goclaw_session_list`, `goclaw_session_preview`, … |
| Configuration | 3 | `goclaw_config_get`, `goclaw_config_apply`, `goclaw_config_patch` |
| Providers | 5 | `goclaw_provider_list`, `goclaw_provider_create`, … |
| MCP Servers | 7 | `goclaw_mcp_server_list`, … |
| Skills | 5 | `goclaw_skill_list`, … |
| Custom Tools | 6 | `goclaw_custom_tool_list`, `goclaw_custom_tool_invoke`, … |
| Cron | 6 | `goclaw_cron_list`, `goclaw_cron_run`, … |
| Teams | 5 | `goclaw_team_list`, … |
| Traces | 2 | `goclaw_trace_list`, `goclaw_trace_get` |
| Channels | 2 | `goclaw_channel_list`, `goclaw_channel_toggle` |
| Memory | 4 | `goclaw_memory_list`, `goclaw_memory_create`, … |

Covers gateway administration: agents, sessions, channels, providers.

## Development

```bash
pnpm -C mcp install
pnpm -C mcp verify   # tsc + tests + build
```

From repo root: `make verify` also runs `pnpm -C mcp verify`.

## License

MIT (fork/rebrand of `goclaw-mcp`)

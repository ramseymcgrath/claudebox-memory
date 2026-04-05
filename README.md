# memory-mcp

Persistent memory for [claudebox](https://github.com/ramseymcgrath/claudebox) sessions. Two components:

1. **Worker** — Cloudflare Worker serving a remote MCP server over Streamable HTTP, backed by D1 (SQLite at edge).
2. **Proxy** — Go stdio MCP proxy that sits between Claude Code and the Worker. Auto-detects namespace, auto-recalls on session start, and exposes simplified tool schemas.

Three tools, no fluff:

- **`recall`** — search memories by namespace. Context auto-loaded at session start.
- **`remember`** — store a decision, pattern, fact, task, or preference.
- **`forget`** — delete a memory by ID.

## Deploy

```bash
# 1. Install deps
npm install

# 2. Create D1 database
npx wrangler d1 create memory-mcp
# Copy the database_id into wrangler.jsonc

# 3. Initialize schema
npm run db:init:remote

# 4. Set auth token
npx wrangler secret put MEMORY_MCP_TOKEN
# Enter a strong random token, e.g.: openssl rand -hex 32

# 5. Deploy
npm run deploy
# Note your worker URL: https://memory-mcp.<account>.workers.dev
```

## Proxy

The proxy (`proxy/`) is a Go binary that Claude Code launches as a stdio MCP server. It handles:

- **Namespace auto-detection**: `MEMORY_NAMESPACE` env var > git remote origin > `/repos/*` scan > `basename /workspace`
- **Auto-recall on session start**: Fires a background `recall` when the session initializes, caches the result
- **Schema rewriting**: Strips `namespace` from tool params in single-repo mode, exposes it as an optional enum in multi-repo mode

### Build

```bash
cd proxy
go build -o memory-mcp-proxy .
```

### Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `MEMORY_MCP_TOKEN` | Yes | — | Service auth token for the Worker |
| `MEMORY_MCP_URL` | No | `https://memory-api.ramseymcgrath.com/mcp` | Remote Worker URL |
| `MEMORY_NAMESPACE` | No | auto-detected | Override namespace detection |

### Connect from claudebox

In `.mcp.json`:

```json
{
  "memory": {
    "type": "stdio",
    "command": "memory-mcp-proxy",
    "args": [],
    "env": {
      "MEMORY_MCP_URL": "https://memory-api.ramseymcgrath.com/mcp",
      "MEMORY_MCP_TOKEN": "${MEMORY_MCP_TOKEN}"
    },
    "trusted": true
  }
}
```

Add `memory-api.ramseymcgrath.com` to the firewall allowlist in `.devcontainer/init-firewall.sh`.

## Namespacing strategy

The `namespace` parameter scopes memories to a project. Conventions:

| Scenario | Namespace |
|---|---|
| Single repo | Repo name: `claudebox` |
| Multi-repo container | Each repo's name: `claudebox`, `claudeproxy` |
| Cross-repo concerns | Shared namespace: `infra` or `platform` |
| Personal preferences | `_global` (convention, not enforced) |

Claude will infer the namespace from the workspace. You can also pin a default in your project's CLAUDE.md:

```markdown
Memory namespace for this project: `claudebox`
```

## Categories

| Category | When to use | Example |
|---|---|---|
| `decision` | Architectural or design choice with rationale | "Chose D1 over KV: need SQL queries and FTS, KV is key-value only" |
| `pattern` | Recurring code or workflow pattern | "All MCP tools in this project return `type: 'text'` content blocks" |
| `fact` | Project-specific fact unlikely to change often | "Auth tokens are stored in the `claude-config` Docker volume" |
| `task` | In-progress work to resume in a future session | "Refactoring auth middleware — extracted JWT validation, still need to update tests" |
| `preference` | User or team coding preference | "Always use `bun` instead of `npm` in this project" |

## Local development

```bash
# Start local dev server
npm run dev

# Initialize local D1
npm run db:init

# Test with MCP inspector
npx @modelcontextprotocol/inspector@latest
# Connect to http://localhost:8787/mcp
```

## How it works

- **Storage**: Cloudflare D1 (SQLite at edge). Memories are rows with namespace, content, category, pinned flag, and timestamps.
- **Search**: SQLite FTS5 with BM25 ranking. Pinned items always sort first. Not semantic search, but fast and dependency-free.
- **Auth**: Bearer token checked before the MCP handler. Set via `MEMORY_MCP_TOKEN` secret.
- **Transport**: Streamable HTTP at `/mcp`. Compatible with Claude Code's HTTP MCP transport.
- **Per-request isolation**: New `McpServer` instance per request per MCP SDK 1.26.0 security requirements.

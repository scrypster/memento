# Memento + Claude Desktop

Connect Memento to Claude Desktop for persistent memory across all your conversations.

## Setup

### Step 1 — Locate your config file

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

### Step 2 — Add the Memento server

Add or merge this into your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento-mcp",
      "args": [],
      "env": {
        "MEMENTO_DATA_PATH": "/path/to/data",
        "MEMENTO_CONNECTIONS_CONFIG": "/path/to/connections.json"
      }
    }
  }
}
```

Replace the paths with your actual binary and data locations.

> The web UI at **http://localhost:6363/integrations → Claude Desktop** generates a pre-filled config with your real paths and a download button.

### Step 3 — Restart Claude Desktop

Fully quit and reopen Claude Desktop. MCP servers are loaded at startup.

### Step 4 — Verify

In a new conversation, Claude should now have access to Memento's 20 memory tools. Try:

```
"Remember that I prefer TypeScript over JavaScript for all new projects."
```

Claude will use `store_memory` to persist this. In your next conversation:

```
"What language should I use for this new project?"
```

Claude will recall your preference automatically.

## Available Tools

Once connected, Claude Desktop has access to the same 20 tools as any MCP client:

- **store_memory** — Persist decisions, preferences, and context
- **recall_memory** — Retrieve by ID, search query, or paginated list
- **find_related** — Hybrid full-text + semantic vector search
- **detect_contradictions** — Find conflicting memories
- **traverse_memory_graph** — Follow entity relationships
- **update_memory_state** — Manage lifecycle (active, archived, superseded)
- And 14 more — see the [full tool list](../../README.md#what-your-ai-gets)

## Full Setup Experience

The web UI at **http://localhost:6363/integrations → Claude Desktop** provides:
- Pre-filled config file with your actual paths
- One-click download button
- Connection test to verify the server is reachable

# Memento + Cursor / Windsurf

Connect Memento to Cursor or Windsurf for persistent AI memory across all your coding sessions.

## Cursor

### Step 1 — Add MCP Server Config

Place this at `~/.cursor/mcp.json` (macOS/Linux) or `%USERPROFILE%\.cursor\mcp.json` (Windows):

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

> The web UI at **http://localhost:6363/integrations → Cursor** generates a pre-filled config with your real paths and a download button.

### Step 2 — Add Cursor Rules (optional but recommended)

Download the Cursor Rules file from the web UI, or create `.cursor/rules/memento.mdc` in your project root:

```markdown
---
description: Use Memento MCP tools to store and recall memories, decisions, and context across sessions.
alwaysApply: true
---

# Memento Memory System

You have access to a persistent memory system via Memento MCP tools. Use these tools proactively to store important context and recall past decisions.

## When to store memories

Store memories when the user:
- States a preference, constraint, or decision
- Describes project architecture or technical choices
- Mentions team members, roles, or relationships
- Sets a requirement or goal for the project

## When to recall memories

Recall memories when:
- Starting a new session or opening a file you haven't seen recently
- The user asks about past decisions or context
- You're about to make a technical recommendation

## Available MCP Tools

- **store_memory** — Store a new memory. Required: `content`. Optional: `tags`, `connection_id`.
- **recall_memory** — Search memories. Required: `query`. Optional: `limit`, `connection_id`.
- **find_related** — Full-text + semantic search. Required: `query`.
- **detect_contradictions** — Find conflicting memories.
- **update_memory_state** — Archive or supersede a memory.
```

Commit this file to version control so the whole team benefits.

### Step 3 — Restart Cursor

Fully quit and reopen Cursor (not just reload window). MCP servers are loaded at startup.

### Step 4 — Per-Project Workspace (optional)

If you have multiple Memento connections, add a `.cursor/rules/memento-workspace.mdc` file to pin a specific workspace for each project. The web UI generates this snippet with a connection selector dropdown.

---

## Windsurf

### Step 1 — Add MCP Server Config

Place this at `~/.codeium/windsurf/mcp_config.json` (macOS/Linux) or `%APPDATA%\Windsurf\User\mcp_config.json` (Windows):

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

> The web UI at **http://localhost:6363/integrations → Windsurf** generates a pre-filled config with a download button.

### Step 2 — Add Windsurf Rules (optional)

Place a rules file at `.windsurf/rules/memento.md` in your project root with the same content as the Cursor Rules above (without the YAML frontmatter).

### Step 3 — Restart Windsurf

Fully restart Windsurf to pick up the new MCP server configuration.

### Step 4 — Per-Project Workspace (optional)

Same as Cursor — use a workspace snippet file to scope Memento to a specific connection per project.

---

## Full Setup Experience

The web UI at **http://localhost:6363/integrations** provides per-client tabs with:
- Pre-filled config files with your actual paths
- One-click download buttons for configs, rules, and workspace snippets
- Connection test to verify the server is reachable
- Per-project workspace scoping with connection selector

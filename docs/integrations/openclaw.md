# Memento + OpenClaw

Give OpenClaw a persistent knowledge graph — so it remembers decisions, relationships, and context across every session.

OpenClaw's built-in memory is flat markdown files. Memento replaces that with a full knowledge graph: entities, relationships, confidence scoring, decay, and hybrid full-text + semantic search. Every conversation builds on the last.

## Setup

### Step 1 — Start Memento

```bash
docker run -d \
  --name memento \
  -p 6363:6363 \
  -v memento-data:/data \
  ghcr.io/scrypster/memento:latest
```

Or download the `memento-mcp` binary from [Releases](https://github.com/scrypster/memento/releases).

### Step 2 — Add to OpenClaw's MCP config

OpenClaw loads MCP servers from `~/.openclaw/mcp.json`. Create or edit that file:

**Using the binary (recommended):**

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento-mcp",
      "args": [],
      "autoUpdate": false,
      "env": {
        "MEMENTO_DATA_PATH": "~/.memento",
        "MEMENTO_USER": "your-name"
      }
    }
  }
}
```

**Using Docker:**

```json
{
  "mcpServers": {
    "memento": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "memento-data:/data",
        "-e", "MEMENTO_USER=your-name",
        "ghcr.io/scrypster/memento:latest",
        "/app/memento-mcp"
      ],
      "autoUpdate": false
    }
  }
}
```

> Set `MEMENTO_USER` to your name or handle. This tags every memory with your identity — useful when sharing a Memento instance with your team.

### Step 3 — Restart OpenClaw

MCP servers are loaded at startup. Fully quit and reopen OpenClaw.

### Step 4 — Verify

In a new session, ask OpenClaw to store something:

```
Remember that we're using PostgreSQL for the main database and Redis for the job queue.
```

OpenClaw will call `store_memory`. In your next session:

```
What database are we using?
```

It will recall the decision automatically — no MEMORY.md required.

## Why Memento over OpenClaw's built-in memory

| | OpenClaw built-in | Memento |
|---|---|---|
| Storage | Flat markdown files | SQLite or PostgreSQL |
| Search | Text file scan | Hybrid full-text + semantic |
| Relationships | None | Knowledge graph (22 entity types) |
| Decay | None | Time-based confidence scoring |
| Team sharing | Per-user files | Shared PostgreSQL instance |
| Cross-tool | OpenClaw only | Claude Code, Cursor, Windsurf, and more |

## Team sharing

If your team runs a shared Memento instance on PostgreSQL, every member points their OpenClaw (and other AI tools) at the same server. Set `MEMENTO_USER` to each person's name so memories are attributed:

- Alice's OpenClaw stores memories tagged `created_by: alice`
- Bob's Cursor stores memories tagged `created_by: bob`
- Either can query: *"What did Alice decide about the auth service?"*

See the [team setup guide](../team-setup.md) for full PostgreSQL configuration.

## Available tools

Once connected, OpenClaw has access to Memento's full tool set:

- **store_memory** — Persist decisions, preferences, and context
- **recall_memory** — Retrieve by ID, search query, or list with filters
- **find_related** — Hybrid full-text + semantic search
- **traverse_memory_graph** — Follow entity relationships
- **detect_contradictions** — Find conflicting memories
- **evolve_memory** — Version a memory when understanding changes
- **update_memory_state** — Manage lifecycle (active, archived, superseded)
- And 13 more — see the [full tool list](../../README.md#what-your-ai-gets)

## Web UI

The Memento web UI at **http://localhost:6363** lets you browse everything OpenClaw has stored, search across sessions, view the knowledge graph, and download pre-filled config files.

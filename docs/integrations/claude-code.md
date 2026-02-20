# Memento + Claude Code

Connect Memento to Claude Code for persistent memory across all your coding sessions.

## Three Integration Levels

| Level | What you get | What to install |
|---|---|---|
| **1. Explicit only** | Tools available but dormant — Claude uses them only when you ask | MCP registration only |
| **2. Skill-assisted** | Invoke `/memento` for focused memory sessions | MCP registration + skill file |
| **3. Always-on** (recommended) | Claude stores decisions and recalls context automatically | MCP registration + skill file + CLAUDE.md snippet |

## Step 1 — Register the MCP Server

> Claude Code v2.x **silently ignores** standalone MCP JSON files (e.g. `~/.claude/mcp.json`). The only supported method is `claude mcp add`, which writes to `~/.claude.json`.

```bash
claude mcp add memento /path/to/memento-mcp --scope user \
  -e MEMENTO_DATA_PATH=~/.memento \
  -e MEMENTO_CONNECTIONS_CONFIG=~/.memento/connections.json
```

Replace `/path/to/memento-mcp` with your actual binary path. The web UI at **http://localhost:6363/integrations** generates a ready-to-paste command with your real paths.

## Step 2 — Verify

```bash
claude mcp list
```

Memento should appear in the list. Then restart Claude Code (close and reopen your terminal or IDE).

Inside a session, type `/mcp` — Memento should show as **connected**.

## Step 3 — Install the Skill File (Level 2+)

Download from the web UI at **Integrations → Claude Code → Download Skill File**, or create it manually:

Place at `~/.claude/skills/memento.md`:

```markdown
# Memento Memory

Persistent cross-session memory for Claude Code via MCP.

## When to use

**Store** when the user states preferences, makes decisions, describes architecture, or shares context that should persist across sessions.

**Recall** before making recommendations, when starting a session, or when the user asks about past context.

## Tools

- **store_memory** — Store a new memory. Required: `content`. Optional: `tags`, `connection_id`.
- **recall_memory** — Search memories. Use `id` for direct lookup, `query` for search, or neither for paginated list.
- **find_related** — Hybrid full-text + semantic search across all memories.
- **update_memory** — Edit content, tags, or metadata.
- **update_memory_state** — Change lifecycle state: `active`, `archived`, `superseded`.
- **detect_contradictions** — Find conflicting memories.
- **explain_reasoning** — Show why memories were retrieved.
- **retry_enrichment** — Re-run entity extraction on a failed memory.
```

## Step 4 — Make It Proactive (Level 3 — Recommended)

Add this to `~/.claude/CLAUDE.md` so Claude uses Memento automatically:

```markdown
## Memento MCP — Persistent Memory

The `memento` MCP server provides persistent cross-session memory. Use these tools proactively — don't wait to be asked.

**Store** (`store_memory`) when the user:
- States a preference or working style ("I prefer X", "always use Y format")
- Makes an architectural or technical decision
- Establishes project context that should survive session restarts
- Explicitly says "remember this" or similar

**Recall** (`recall_memory` or `find_related`) when:
- Starting a session for a known project — query for relevant context before diving in
- About to make a recommendation — check for existing preferences first
- The user asks about past decisions, choices, or "what did we decide about X"
- Something seems like it may have been discussed in a prior session

**Don't store:** transient debug output, in-progress exploration, or anything session-specific that won't matter next time.

Memories are searchable immediately after storing. Enrichment (entity/relationship extraction) runs asynchronously via local Ollama.
```

### Without the snippet vs. with it

| Without | With |
|---|---|
| Tools available but dormant | Decisions stored automatically |
| Must say "remember this" every time | Preferences recalled at session start |
| No automatic context recall | Works across all projects globally |

## Per-Project Workspace Scoping

If you have multiple Memento connections (e.g., separate workspaces for different clients), add a `CLAUDE.md` in your project root to pin a specific connection:

```markdown
## Memento — Project Workspace

This project uses the **my-project** Memento workspace.

When using Memento MCP tools in this project, always pass `connection_id: "my-project"` to
scope reads and writes to the correct workspace.
```

You can also set a global default by adding `-e MEMENTO_DEFAULT_CONNECTION=my-project` to your `claude mcp add` command.

## Troubleshooting

**MCP tools not showing in Claude Code**
Run `claude mcp list` to confirm registration. If missing, re-run `claude mcp add` and restart.

**Server appears but tools fail**
Check that `memento-mcp` binary is executable and `MEMENTO_DATA_PATH` directory exists.

**Remove registration**
```bash
claude mcp remove memento --scope user
```

## Full Setup Experience

The web UI at **http://localhost:6363/integrations → Claude Code** provides:
- Pre-filled CLI command with your actual paths
- Skill file and CLAUDE.md snippet downloads
- Connection test button
- Per-project workspace snippet generator

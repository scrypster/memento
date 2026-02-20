# Team Setup — Shared AI Memory

Give your entire engineering team a shared knowledge graph. When one person's AI learns something, everyone's AI can recall it.

## How it works

Memento stores every memory with a `created_by` field — auto-detected from your git config, or set via `MEMENTO_USER`. When multiple people point their AI tools at the same Memento instance (backed by a shared PostgreSQL database), the team's collective knowledge lives in one place.

- Alice's Claude Code stores an architecture decision → tagged `created_by: alice`
- Bob opens Cursor the next day → asks about the auth service → gets Alice's decision
- Either teammate can ask: *"What did Alice decide about the auth service this week?"*

## Setup

### Step 1 — Deploy a shared Memento instance

Run Memento with PostgreSQL as the storage backend. One Postgres database, shared by the team:

```bash
docker run -d \
  --name memento \
  -p 6363:6363 \
  -e MEMENTO_STORAGE_ENGINE=postgres \
  -e MEMENTO_DATABASE_URL=postgres://memento:password@your-db-host:5432/memento \
  ghcr.io/scrypster/memento:latest
```

Or use a managed PostgreSQL service (RDS, Supabase, Railway, Neon, etc.) — Memento runs migrations automatically on startup.

### Step 2 — Set MEMENTO_USER per person

Each team member sets their name so memories are attributed correctly.

**Option A: Environment variable (recommended)**

Add to `~/.zshrc` or `~/.bashrc`:
```bash
export MEMENTO_USER=alice
```

**Option B: Git config (automatic)**

Memento reads `git config --get user.name` if `MEMENTO_USER` is not set. If your team already has consistent git user names, nothing extra is needed.

**Option C: MCP config env**

Set it per-connection in your MCP config file:
```json
{
  "mcpServers": {
    "team-memory": {
      "command": "/path/to/memento-mcp",
      "env": {
        "MEMENTO_USER": "alice",
        "MEMENTO_DATA_PATH": "~/.memento"
      }
    }
  }
}
```

### Step 3 — Configure connections

Each team member connects their AI tools to the shared Memento instance. In Claude Code:

```bash
claude mcp add team-memory -- /path/to/memento-mcp \
  -e MEMENTO_DATABASE_URL=postgres://memento:password@your-db-host:5432/memento \
  -e MEMENTO_USER=alice
```

The web UI at **http://your-memento-host:6363/integrations** generates ready-to-paste configs for Claude Desktop, Cursor, Windsurf, and OpenClaw.

## Personal + team connections

You can run both:

| Connection | Database | Purpose |
|---|---|---|
| `personal` | Local SQLite | Private notes, preferences, experiments |
| `team-shared` | Shared PostgreSQL | Architecture decisions, conventions, project context |

Configure two connections in your MCP config and Claude will use the right one based on what you're storing.

## Querying by author and time

With `MEMENTO_USER` set, every memory is attributed. Your AI can filter by author and time:

```
What did Bob work on yesterday?
→ recall_memory(created_by="bob", created_after="2024-01-14T00:00:00Z")

What decisions has the team made about the payments service?
→ find_related(query="payments service decisions", domain="architecture")

Show me everything Alice stored this sprint
→ recall_memory(created_by="alice", created_after="2024-01-08T00:00:00Z")
```

## Access control

PostgreSQL handles access control at the database level. Options:

- **Single shared user** — simplest setup, full team access to everything
- **Per-person PostgreSQL users** — each person gets their own PG credentials, DBA controls table-level permissions
- **Schema per team** — create separate schemas for different teams or projects within the same Postgres instance

For most small engineering teams, a single shared user with a strong password and network-level access controls (VPN, private subnet) is sufficient.

## Domain-based organization

Use the `domain` field to organize shared memories by project or topic:

```
store_memory("We're using event sourcing for the orders service", domain="architecture")
store_memory("Deploy to staging before 3pm on Fridays", domain="devops-conventions")
store_memory("Alice owns the payments module", domain="team")
```

Team members can then search within a domain:
```
recall_memory(query="orders service", domain="architecture")
```

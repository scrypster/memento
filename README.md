<p align="center">
  <img src="docs/screenshots/banner.png" width="300" alt="Memento — Remember Everything, Forget Nothing" />
</p>

# Memento

> **Give your AI tools a persistent memory — so every session starts where the last one left off.**
> *Inspired by Nolan's Memento — except this time, the graph tattoos actually stick.*

[![Version](https://img.shields.io/badge/version-v0.1.0--alpha-orange)](CHANGELOG.md)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-compose-2496ED?logo=docker)](docker-compose.yml)
[![MCP](https://img.shields.io/badge/MCP-compatible-8B5CF6)](https://modelcontextprotocol.io)

---

## 🎯 The Problem

You're deep in a codebase with an AI assistant that's finally been helpful. It knows your architecture, your naming conventions, why you chose that approach for the auth service. You close the tab.

Next session: it remembers nothing.

**Memento fixes the critical problem of context loss in AI-assisted development:**

- **🧠 Zero session memory** — your AI starts fresh every single time, no matter how much ground you covered
- **❓ "Why did we do it this way?"** — architectural decisions made last month are invisible to your tools today
- **🔄 Repeated explanations** — you re-explain your stack, your patterns, your constraints, session after session
- **👶 Onboarding friction** — new team members (and new AI sessions) have no access to established context
- **🤖 AI integration gap** — current AI tools have no persistent memory of *your specific* project and decisions

---

## ✨ The Solution: A Persistent Knowledge Layer

![Dashboard](docs/screenshots/dashboard.png)

Memento runs on your machine and connects to any MCP-compatible AI tool. Every session, your AI already knows what you've built, who's involved, what you decided, and why.

**No manual tagging. No cloud. No subscriptions. Your data stays on your machine.**

> Works with **Claude Code**, **Claude Desktop**, **Cursor**, **Windsurf**, **OpenClaw**, and any MCP-compatible client.

---

## 🚀 Quick Start

```bash
git clone https://github.com/scrypster/memento.git
cd memento
./launch.sh
```

The launch script asks **Docker or Local?** and handles everything:

- **Docker** — checks Docker/Compose, builds images, starts containers, waits for health
- **Local** — checks Go, Ollama, required models, builds binaries, offers to start

> First run downloads Ollama models `qwen2.5:7b` + `nomic-embed-text` (~5 GB). After that, starts in seconds with no external calls ever.

### Manual: Docker

```bash
docker compose up -d
```

Open **http://localhost:6363** and follow the setup guide.

### Manual: Go binary

```bash
go build -o memento-web ./cmd/memento-web/ && ./memento-web
```

---

## Connect your tools

The web UI at port 6363 generates all configs and shows you exactly where to place them.

### Claude Code

```bash
claude mcp add memento -- /path/to/memento-mcp \
  -e MEMENTO_DATA_PATH=~/.memento
```

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento-mcp",
      "env": { "MEMENTO_DATA_PATH": "~/.memento" }
    }
  }
}
```

### Cursor / Windsurf

Add to `.cursor/mcp.json` or `.windsurf/mcp.json`:

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento-mcp",
      "env": { "MEMENTO_DATA_PATH": "~/.memento" }
    }
  }
}
```

### OpenClaw

Memento is a first-class integration in [OpenClaw](https://openclaw.io). Add it as an MCP server in your OpenClaw workspace config and every agent in your swarm shares the same persistent memory — decisions made by one agent are immediately available to all others. No more context re-injection between agent calls.

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento-mcp",
      "env": { "MEMENTO_DATA_PATH": "~/.memento" }
    }
  }
}
```

### Generic MCP

Any MCP-compatible client uses the same pattern — command path + `MEMENTO_DATA_PATH` env var. The web UI generates a ready-to-paste config for each client.

---

## What it looks like in practice

### The knowledge graph

![Graph Explorer](docs/screenshots/graph.png)

Every entity Memento extracts gets wired into a graph. Your AI already knows that Python is used by Kate and Rachel, that it relates to Machine Learning and Pandas — because Memento mapped it automatically while you worked.

### Auto-extracted entities — zero manual input

![Entities](docs/screenshots/entities.png)

People, projects, tools, organizations, languages, APIs — extracted automatically from your AI conversations. 143 entities from 110 memories, no tagging required.

### Relationship intelligence

![Relationships](docs/screenshots/relationships.png)

Your AI knows who `works_on` what, which tools `depend_on` which services, and what the current state of each decision is — with confidence scores and timestamps.

### One-click integration setup

![Integrations](docs/screenshots/integrations.png)

The web UI generates your client config and tells you exactly where to drop it. Cursor, Windsurf, Claude Code, Claude Desktop — all covered.

---

## The workflow

### Before starting work
Your AI already has context. No re-explaining your stack, team, or constraints.

```
"What database decisions have we made?"
→ Memento surfaces: PostgreSQL for main storage (chosen for pgvector), Redis for sessions (security decision, March 2025)
```

### During development
Decisions get stored as you make them — automatically.

```
"We're switching the auth service to JWT"
→ Memento stores the decision, extracts entities, maps relationships to existing auth memories
```

### Across sessions
The next session starts with everything intact.

```
New session, fresh tab — your AI opens knowing your full project context, team, and decision history
```

---

## What your AI gets

Once connected, your AI has **20 tools** it can call automatically — no prompting required:

### Core memory operations

| Tool | What it does |
|---|---|
| `store_memory` | Persist a decision or piece of context — enrichment happens async, returns in <10ms |
| `recall_memory` | Retrieve memories by ID, natural-language query, or paginated list with filters |
| `find_related` | Hybrid search: full-text + semantic vector + RRF ranking |
| `update_memory` | Edit content, tags, or metadata of an existing memory |
| `forget_memory` | Soft-delete a memory (with grace period) or hard-delete permanently |

### Search and intelligence

| Tool | What it does |
|---|---|
| `traverse_memory_graph` | Follow entity relationships to discover contextually connected memories (multi-hop BFS) |
| `detect_contradictions` | Find conflicting relationships, superseded-but-active memories, temporal impossibilities |
| `explain_reasoning` | Surface why specific memories were retrieved for a query |
| `get_session_context` | "Where did I leave off?" — recent memories grouped by topic |

### Memory lifecycle

| Tool | What it does |
|---|---|
| `update_memory_state` | Move through lifecycle: `planning → active → paused / blocked / completed → archived` |
| `evolve_memory` | Create a new version that supersedes the old one — preserves full history |
| `consolidate_memories` | LLM-assisted merge of multiple related memories into one coherent record |
| `get_evolution_chain` | View the full version history of a memory from original to latest |

### Soft delete and recovery

| Tool | What it does |
|---|---|
| `restore_memory` | Recover a soft-deleted memory |
| `list_deleted_memories` | Browse soft-deleted memories that can still be restored |
| `retry_enrichment` | Re-run entity extraction on a memory that previously failed |

### Project management

| Tool | What it does |
|---|---|
| `create_project` | Create a project memory with optional pre-created phases |
| `add_project_item` | Add epics, phases, tasks, steps, or milestones under a project |
| `get_project_tree` | Retrieve the full nested hierarchy of a project |
| `list_projects` | List all projects, optionally filtered by lifecycle state |

**Store returns in <10ms.** Enrichment — entity extraction, relationship mapping, embedding generation — runs asynchronously. Your AI is never blocked.

---

## Why Memento vs. everything else

### vs. Just using the AI's context window
- **Persistent**: Survives session ends, tab closes, computer restarts
- **Searchable**: Find any decision from any past session by meaning, not keyword
- **Cross-session**: Build context over weeks and months, not just within one chat

### vs. Writing docs / wikis
- **Automatic**: No manual effort — it captures context as you work
- **Connected**: Relationships between concepts vs. isolated pages
- **Usage-driven**: Frequently accessed knowledge rises; stale context fades

### vs. Code comments
- **Survives refactoring**: Business reasoning preserved even as code changes
- **Cross-project**: Not locked inside one file or repo
- **AI-native**: Designed to be queried by LLMs, not just humans

### vs. Slack / Notion
- **Structured**: Enforced knowledge graph vs. message chaos or free-form text
- **AI-queryable**: Your tools can search it programmatically, not just you
- **Private**: Runs on your machine, not a third-party SaaS

---

## How it works

```
┌─────────────────────────────────────────────────────┐
│  Your AI tool (Cursor / Claude Code / Windsurf / …) │
└─────────────────────────┬───────────────────────────┘
                          │  MCP (JSON-RPC 2.0 over stdio)
┌─────────────────────────▼───────────────────────────┐
│                   MCP Server                        │
│   store · recall · find_related · contradictions…   │
└─────────────────────────┬───────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────┐
│                Memory Engine                        │
│  ┌──────────────────────────────────────────────┐  │
│  │           Enrichment Pipeline                │  │
│  │  entity extraction → relationship mapping    │  │
│  │  → semantic embeddings → contradiction check │  │
│  └──────────────────────────────────────────────┘  │
└──────────────────┬──────────────────────────────────┘
                   │
       ┌───────────┴───────────┐
       │                       │
┌──────▼──────┐       ┌────────▼────────┐
│   SQLite    │       │  PostgreSQL     │
│  FTS5 index │       │  + pgvector     │
│  (default)  │       │  (scale-out)    │
└─────────────┘       └─────────────────┘
```

---

## Features

**No cloud required**
- Ollama runs locally — default setup never makes an external network call
- SQLite database is a single file you own: `~/.memento/memento.db`
- Swap to OpenAI or Anthropic when you want stronger extraction — opt-in only

**Search that actually works**
- FTS5 full-text + semantic vector search fused with Reciprocal Rank Fusion (RRF)
- Finds what you mean, not just what you typed

**Knowledge graph**
- Extracts 22 entity types: people, projects, tools, languages, APIs, databases, concepts, and more
- Maps 44 relationship types with confidence scores
- Interactive graph explorer in the web UI

**Memory that ages gracefully**
- Lifecycle states: `planning → active → paused | blocked | completed | cancelled → archived`
- Decay scoring — stale context loses ranking weight naturally
- Access-frequency boosting — memories you recall often stay prominent

**Production-ready backends**
- SQLite (zero deps, CGo-free) for personal/local use
- PostgreSQL + pgvector + ivfflat index for team or production deployments

**Multi-connection isolation**
- Separate memory namespaces per project, client, or workspace
- Route MCP calls to different connections with a single env var

**Observable**
- Web UI: live enrichment queue, entity browser, relationship explorer, graph visualizer
- Tracks unrecognized LLM entity types so you can expand your taxonomy over time

---

## LLM providers

| Provider | Setup | Use when |
|---|---|---|
| **Ollama** (default) | `docker compose up` — automatic | Privacy first, no API costs, fully offline |
| **OpenAI** | Set `MEMENTO_LLM_PROVIDER=openai` + API key | Stronger extraction quality, cloud OK |
| **Anthropic** | Set `MEMENTO_LLM_PROVIDER=anthropic` + API key | Strongest reasoning, cloud OK |

Switch providers per connection — different projects can use different LLMs.

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `MEMENTO_PORT` | `6363` | Web UI and REST API port |
| `MEMENTO_STORAGE_ENGINE` | `sqlite` | `sqlite` or `postgres` |
| `MEMENTO_DATA_PATH` | `./data` | SQLite database directory |
| `MEMENTO_LLM_PROVIDER` | `ollama` | `ollama`, `openai`, or `anthropic` |
| `MEMENTO_OLLAMA_URL` | `http://localhost:11434` | Ollama API endpoint |
| `MEMENTO_OLLAMA_MODEL` | `qwen2.5:7b` | Extraction model |
| `MEMENTO_EMBEDDING_MODEL` | `nomic-embed-text` | Embedding model |
| `MEMENTO_OPENAI_API_KEY` | — | OpenAI API key |
| `MEMENTO_ANTHROPIC_API_KEY` | — | Anthropic API key |
| `MEMENTO_DEFAULT_CONNECTION` | — | Default connection name for multi-workspace isolation |
| `MEMENTO_CONNECTIONS_CONFIG` | — | Path to `connections.json` for multi-workspace setup |
| `MEMENTO_BACKUP_ENABLED` | `false` | Automated backups |
| `MEMENTO_BACKUP_INTERVAL` | `24h` | Backup frequency |

### PostgreSQL

```bash
docker compose --profile postgres up -d
```

```bash
MEMENTO_STORAGE_ENGINE=postgres
MEMENTO_DATABASE_URL=postgres://memento:memento_dev_password@localhost:5433/memento
```

---

## Project structure

```
memento/
├── cmd/
│   ├── memento-mcp/        # MCP server binary — connect this to your AI client
│   ├── memento-web/        # Web dashboard — entity browser, graph explorer, settings
│   └── memento-setup/      # Interactive setup wizard
├── internal/
│   ├── api/mcp/            # MCP JSON-RPC server — 20 tool handlers
│   ├── engine/             # Memory engine, enrichment pipeline, async workers
│   ├── llm/                # Ollama, OpenAI, Anthropic + circuit breaker
│   └── storage/
│       ├── sqlite/         # SQLite with FTS5 and hybrid vector search
│       └── postgres/       # PostgreSQL with pgvector and ivfflat index
├── web/
│   ├── handlers/           # HTMX handlers
│   ├── templates/          # Dashboard, graph, entities, settings, integrations
│   └── static/templates/   # MCP config snippets generated per client
├── migrations/             # SQL schema migrations
└── docker-compose.yml
```

---

## Contributing

Issues and PRs welcome. Open an issue before starting significant work.

```bash
go test ./...

go build -o memento-mcp ./cmd/memento-mcp/
go build -o memento-web ./cmd/memento-web/
go build -o memento-setup ./cmd/memento-setup/
```

---

## License

MIT — see [LICENSE](LICENSE).

---

## Built by

**[MJ Bonanno](https://mjbonanno.com)** — software architect and founder of **[Scrypster](https://scrypster.com)**.

---

**Remember everything. Forget nothing.** 🧠✨

*Unlike Leonard Shelby, your context is here to stay — searchable, versioned, and backed by a knowledge graph that never fades.*

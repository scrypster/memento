<p align="center">
  <img src="docs/screenshots/banner.png" width="300" alt="Memento — Remember Everything, Forget Nothing" />
</p>

# Memento

> **Give your AI tools a persistent memory — so every session starts where the last one left off.**

[![Version](https://img.shields.io/badge/version-v0.1.0--alpha-orange)](CHANGELOG.md)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-compose-2496ED?logo=docker)](docker-compose.yml)
[![MCP](https://img.shields.io/badge/MCP-compatible-8B5CF6)](https://modelcontextprotocol.io)

Your AI starts fresh every session. Memento fixes that.

It runs on your machine, connects to any MCP-compatible AI tool, and builds a persistent knowledge graph from your conversations — entities, relationships, decisions, and context that survive every session restart.

**No cloud. No API keys required. No subscriptions. Your data stays on your machine.**

---

## Quick Start

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

## First 60 Seconds

Once running, connect your AI tool and try this:

```
You: "We're using PostgreSQL for the main database — chose it for pgvector support."

→ Memento stores the decision, extracts entities (PostgreSQL, pgvector),
  maps the relationship (PostgreSQL → depends_on → pgvector), and indexes
  everything for search. Returns in <10ms.
```

Close the tab. Open a new session.

```
You: "What database are we using?"

→ Your AI already knows: "PostgreSQL — you chose it for pgvector support."
  No re-explaining. No context window tricks. It just remembers.
```

Behind the scenes, Memento built this automatically:

![Graph Explorer](docs/screenshots/graph.png)

Every entity gets wired into a knowledge graph — people, tools, projects, decisions — with confidence scores and timestamps.

---

## Connect Your Tools

Open **http://localhost:6363/integrations** — the web UI generates configs, download buttons, and connection testing for every client:

![Integrations](docs/screenshots/integrations.png)

| Client | Setup |
|---|---|
| **Claude Code** | `claude mcp add memento -- /path/to/memento-mcp -e MEMENTO_DATA_PATH=~/.memento` |
| **Claude Desktop** | Download config → drop in `~/Library/Application Support/Claude/` |
| **Cursor** | Download config → drop in `.cursor/mcp.json` + optional Cursor Rules file |
| **Windsurf** | Download config → drop in `.codeium/windsurf/mcp_config.json` |
| **OpenClaw** | Add to `~/.openclaw/mcp.json` under `mcpServers` — same pattern as Claude Desktop |
| **Generic MCP** | Any MCP client — same pattern: command path + `MEMENTO_DATA_PATH` env var |

> The integrations page generates ready-to-paste configs with your actual binary paths and data directories. It also has connection testing, troubleshooting, and per-project workspace scoping.

### Make Claude Code proactive (recommended)

The MCP connection makes tools *available*, but Claude won't use them automatically. Add this to `~/.claude/CLAUDE.md` to make Claude store decisions and recall context without being asked:

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

> The web UI at **Integrations → Claude Code → Make it proactive** generates a version with your specific paths and connection settings, plus a download button.

See the full integration guides: [Claude Code](docs/integrations/claude-code.md) | [Claude Desktop](docs/integrations/claude-desktop.md) | [Cursor & Windsurf](docs/integrations/cursor-windsurf.md) | [OpenClaw](docs/integrations/openclaw.md)

### Team memory — shared knowledge across your whole engineering team

Point everyone's AI tools at the same Memento instance and your team's decisions, conventions, and context become shared knowledge — queryable by anyone, attributable to anyone.

Every memory is tagged with who stored it. Memento auto-detects this from your git config, or you can set it explicitly:

```bash
export MEMENTO_USER=alice   # or set in your shell profile
```

Or in your MCP config:
```json
"env": { "MEMENTO_USER": "alice" }
```

Once set, you can ask:

```
What did Bob decide about the auth service this week?
recall_memory(created_by="bob", created_after="2024-01-14T00:00:00Z")
```

**Setup:** Each teammate runs Memento pointing at the same PostgreSQL database. Personal context stays personal (use a separate personal connection). Shared architectural decisions, conventions, and project context go into the shared connection.

See the [team setup guide](docs/team-setup.md) for full PostgreSQL configuration.

---

## What Your AI Gets

Once connected, your AI has **20 tools** it can call — no prompting required:

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

## What It Looks Like

### Auto-extracted entities — zero manual input

![Entities](docs/screenshots/entities.png)

People, projects, tools, organizations, languages, APIs — extracted automatically from your AI conversations. No tagging required.

### Relationship intelligence

![Relationships](docs/screenshots/relationships.png)

Your AI knows who `works_on` what, which tools `depend_on` which services, and what the current state of each decision is — with confidence scores and timestamps.

### The dashboard

![Dashboard](docs/screenshots/dashboard.png)

Live enrichment queue, entity browser, relationship explorer, and graph visualizer — all in the web UI.

---

## Why Memento

### vs. Mem0

Mem0 requires cloud API keys and a paid plan for production use. Memento runs entirely on your machine with Ollama — no API keys, no cloud, no per-memory pricing. Memento also ships a full web UI with graph visualization, entity browser, and one-click integration setup. Mem0 has no web interface.

### vs. Zep / Graphiti

Zep requires Neo4j or FalkorDB for its knowledge graph. Memento uses SQLite (zero deps) or PostgreSQL — no graph database to manage. Zep's open-source version is limited; the full feature set requires Zep Cloud.

### vs. Built-in AI memory (ChatGPT, Claude)

Built-in memory is a flat list of facts with no relationships, no search, no graph, and no way to export or control your data. Memento gives you a structured knowledge graph you own, with hybrid search and full lifecycle management.

### vs. Writing docs or wikis

Memento captures context automatically as you work — no manual effort. It builds relationships between concepts instead of isolated pages, and it's designed to be queried by LLMs, not just humans.

---

## How It Works

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

**Runs entirely offline**
- Ollama runs locally — default setup never makes an external network call
- SQLite database is a single file you own: `~/.memento/memento.db`
- Swap to OpenAI or Anthropic when you want stronger extraction — opt-in only

**Hybrid search**
- FTS5 full-text + semantic vector search fused with Reciprocal Rank Fusion (RRF)
- Finds what you mean, not just what you typed

**Knowledge graph**
- Extracts 22 entity types: people, projects, tools, languages, APIs, databases, concepts, and more
- Maps 44 relationship types with confidence scores
- Interactive graph explorer in the web UI

**Memory lifecycle**
- Lifecycle states: `planning → active → paused | blocked | completed | cancelled → archived`
- Decay scoring — stale context loses ranking weight naturally
- Access-frequency boosting — memories you recall often stay prominent

**Production-ready backends**
- SQLite (zero deps, CGo-free) for personal/local use
- PostgreSQL + pgvector + ivfflat index for team or production deployments

**Multi-connection isolation**
- Separate memory namespaces per project, client, or workspace
- Route MCP calls to different connections with a single env var

**Web UI**
- Dashboard with live enrichment queue, entity browser, relationship explorer, graph visualizer
- One-click integration setup for every supported client
- Connection testing, CLAUDE.md generation, Cursor Rules download
- Tracks unrecognized LLM entity types so you can expand your taxonomy over time

---

## LLM Providers

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

## Project Structure

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
├── docs/
│   └── integrations/       # Per-client integration guides
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

*Remember everything. Forget nothing.*
*Unlike Leonard Shelby, your context is here to stay — searchable, versioned, and backed by a knowledge graph that never fades.*

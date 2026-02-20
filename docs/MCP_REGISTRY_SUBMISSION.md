# MCP Registry Submission Guide

## Status

Memento is **not yet listed** on the official MCP Registry at `registry.modelcontextprotocol.io`.

**Key finding:** Neither Mem0/OpenMemory nor Zep/Graphiti are listed either. The "persistent memory" category has very few entries. Listing now gives Memento first-mover advantage.

> There is a different project called "memento-protocol" (`io.github.myrakrusemark/memento-protocol`) — a SaaS product requiring an API key. Completely different from our local/self-hosted Memento. Our namespace would be `io.github.scrypster/memento` — no conflict.

## How the MCP Registry Works

The registry is a **metaregistry** — it stores metadata about MCP servers, not actual code. It points to where your package is hosted (npm, PyPI, Docker Hub, GitHub Releases, etc.).

Submission uses a CLI tool called `mcp-publisher`. No PR. No form.

## Supported Package Types

| Type | Registry | Best for |
|---|---|---|
| npm | npmjs.org | JavaScript/TypeScript |
| PyPI | pypi.org | Python |
| NuGet | nuget.org | .NET |
| Docker/OCI | Docker Hub, GHCR, GAR, ACR | **Go binaries (recommended for us)** |
| MCPB | GitHub/GitLab Releases | **Go binaries (alternative)** |

## Recommended Path: Docker/OCI on GHCR

Since Memento is a Go binary, Docker image on GitHub Container Registry is the cleanest path.

### Prerequisites

1. Push Memento repo to GitHub at `github.com/scrypster/memento` (must be public)
2. GitHub Container Registry access (comes with any GitHub account)

### Step 1: Add OCI label to Dockerfile

```dockerfile
LABEL io.modelcontextprotocol.server.name="io.github.scrypster/memento"
```

### Step 2: Build and push Docker image

```bash
docker build -t ghcr.io/scrypster/memento:latest .
docker push ghcr.io/scrypster/memento:latest
```

### Step 3: Create `server.json` in repo root

```json
{
  "$schema": "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
  "name": "io.github.scrypster/memento",
  "title": "Memento",
  "description": "Persistent memory MCP server for AI tools. Local-first knowledge graph with hybrid search (FTS5 + semantic vector + RRF), 22 entity types, 44 relationship types, and 20 MCP tools. Runs entirely offline with Ollama — no cloud, no API keys, no subscriptions.",
  "websiteUrl": "https://github.com/scrypster/memento",
  "repository": {
    "url": "https://github.com/scrypster/memento",
    "source": "github"
  },
  "version": "0.1.0",
  "packages": [
    {
      "registryType": "oci",
      "identifier": "ghcr.io/scrypster/memento:0.1.0",
      "transport": {
        "type": "stdio"
      }
    }
  ]
}
```

### Step 4: Install mcp-publisher and authenticate

```bash
# macOS
brew install mcp-publisher

# Or direct download
curl -L "https://github.com/modelcontextprotocol/registry/releases/latest/download/mcp-publisher_darwin_arm64.tar.gz" | tar xz mcp-publisher && sudo mv mcp-publisher /usr/local/bin/

# Authenticate with GitHub
mcp-publisher login github
```

### Step 5: Publish

```bash
mcp-publisher publish
```

### Step 6: Verify

```bash
curl "https://registry.modelcontextprotocol.io/v0/servers?search=io.github.scrypster/memento"
```

## Alternative: MCPB via GitHub Releases

If you prefer standalone binaries (no Docker for users):

1. Create a GitHub Release with cross-compiled Go binaries
2. Bundle as `.mcpb` format
3. Use `registryType: "mcpb"` with the release download URL
4. Include `fileSha256` in server.json (computed with `openssl dgst -sha256`)

## Namespace Rules

- With GitHub auth: server name must start with `io.github.<github-org>/`
- For `com.scrypster/memento`: requires DNS-based authentication (add later)
- Our namespace: `io.github.scrypster/memento`

## What the Listing Looks Like

Once published, Memento appears in registry searches and compatible client UIs. Example from Letta's listing:

```json
{
  "name": "com.letta/memory-mcp",
  "description": "MCP server for AI memory management using Letta",
  "version": "2.0.2",
  "packages": [{
    "registryType": "npm",
    "identifier": "@letta-ai/memory-mcp"
  }]
}
```

## Checklist

- [ ] Push repo to GitHub (public)
- [ ] Add OCI label to Dockerfile
- [ ] Build and push Docker image to GHCR
- [ ] Create `server.json`
- [ ] Install `mcp-publisher`
- [ ] Authenticate with GitHub
- [ ] Publish to registry
- [ ] Verify listing
- [ ] Add registry badge to README

## References

- [MCP Registry](https://registry.modelcontextprotocol.io)
- [Quickstart: Publish an MCP Server](https://modelcontextprotocol.io/registry/quickstart)
- [Package Types](https://github.com/modelcontextprotocol/registry/blob/main/docs/modelcontextprotocol-io/package-types.mdx)
- [Server.json Format](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/server-json/generic-server-json.md)
- [mcp-publisher CLI](https://github.com/modelcontextprotocol/registry)

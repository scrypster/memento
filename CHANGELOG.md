# Changelog

All notable changes to Memento are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [0.1.0-alpha] — 2026-02-18

First public alpha release.

### Added
- Persistent memory storage with SQLite (zero-dependency) and PostgreSQL (with pgvector) backends
- MCP (Model Context Protocol) server with 8 tools: store_memory, recall_memory, find_related, update_memory, update_memory_state, detect_contradictions, explain_reasoning, retry_enrichment
- Multi-provider LLM abstraction: Ollama (local), OpenAI, Anthropic
- Async enrichment pipeline: entity extraction, relationship mapping, semantic embeddings
- Hybrid search: FTS5 full-text search + vector similarity with Reciprocal Rank Fusion (RRF)
- Web UI with memory graph visualization (Cytoscape.js)
- Multi-workspace support via connection manager
- Circuit breaker protection on all LLM calls
- Docker Compose setup with automatic Ollama model provisioning
- Interactive setup wizard with Claude Code MCP auto-registration
- Contradiction detection across memory graph
- Memory lifecycle state machine (planning → active → archived)
- Decay scoring for memory relevance over time
- Integration templates for Claude Code, Claude Desktop, Cursor, OpenClaw

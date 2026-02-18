# Contributing to Memento

Thank you for your interest in contributing! This guide covers everything you need to get started.

## Quick Start

```bash
# Clone the repo
git clone https://github.com/scrypster/memento.git
cd memento

# Install dependencies
go mod download

# Run tests
make test-unit

# Build binaries
make build-all

# Start development environment
docker compose up -d
```

## Development Setup

### Prerequisites

- Go 1.23+
- Docker and Docker Compose
- Ollama (for local LLM enrichment): https://ollama.ai

### Local Development

```bash
# Build and run the web server
go run ./cmd/memento-web/

# Build and run the MCP server (for testing)
go run ./cmd/memento-mcp/

# Run the setup wizard
go run ./cmd/memento-setup/
```

### Environment Variables

Copy `.env.example` to `.env` and customize as needed. Key variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `MEMENTO_PORT` | `6363` | Web UI port |
| `MEMENTO_DATA_PATH` | `./data` | Database directory |
| `MEMENTO_LLM_PROVIDER` | `ollama` | LLM provider (ollama/openai/anthropic) |
| `MEMENTO_OLLAMA_URL` | `http://localhost:11434` | Ollama API URL |

## Project Structure

```
├── cmd/
│   ├── memento-web/     # Web UI + REST API server
│   ├── memento-mcp/     # MCP stdio server
│   └── memento-setup/   # Interactive setup wizard
├── internal/
│   ├── api/mcp/         # MCP protocol implementation
│   ├── config/          # Configuration management
│   ├── connections/     # Multi-workspace connection manager
│   ├── engine/          # Memory engine + enrichment pipeline
│   ├── llm/             # LLM provider abstraction (Ollama, OpenAI, Anthropic)
│   ├── server/          # HTTP server
│   └── storage/         # Storage backends (SQLite, PostgreSQL)
├── web/
│   ├── handlers/        # HTTP request handlers
│   ├── static/          # Static assets + integration templates
│   └── templates/       # HTML templates
└── docs/                # Documentation
```

## Making Changes

### Adding a New LLM Provider

1. Implement the `llm.TextGenerator` and/or `llm.EmbeddingGenerator` interfaces
2. Register your provider in `internal/llm/factory.go`
3. Add tests in `internal/llm/your_provider_test.go`

### Adding a New Storage Backend

1. Implement the storage interfaces in `internal/storage/interfaces.go`
2. Add your backend under `internal/storage/yourbackend/`
3. Register it in `internal/connections/manager.go`

### Adding a New MCP Tool

1. Add the tool definition in `internal/api/mcp/server.go` `buildToolsList()`
2. Add the handler method on `Server`
3. Wire it in `handleToolsCall()`
4. Add tests in `internal/api/mcp/server_test.go`

## Testing

```bash
# Run unit tests (fast, no external dependencies)
make test-unit

# Run all tests including integration
make test

# Run with race detector
go test -race ./...

# Run with coverage
make test-coverage
```

### Test Tiers

- **Unit tests**: No external dependencies, run with `-short` flag
- **Integration tests**: Require Ollama (`make test-integration`)
- **Load tests**: Long-running stress tests (`make test-load`)

## Code Style

- Follow standard Go conventions (`gofmt`, `golangci-lint`)
- Write table-driven tests
- Use `testify` for assertions
- Add package-level doc comments
- Prefer `log/slog` for structured logging in new code

## Pull Request Process

1. Fork the repo and create a feature branch
2. Make your changes with tests
3. Ensure `make test-unit` passes
4. Ensure `go vet ./...` is clean
5. Submit a PR with a clear description

## Reporting Issues

Please use [GitHub Issues](https://github.com/scrypster/memento/issues) for:
- Bug reports (include OS, Go version, steps to reproduce)
- Feature requests
- Documentation improvements

## License

By contributing, you agree your contributions are licensed under the [MIT License](LICENSE).

.PHONY: help build build-backup build-setup build-all setup up down restart logs ps clean test test-unit test-integration test-bench test-load

# Default target
help:
	@echo "Memento - Persistent Memory System"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make build       - Build Docker images"
	@echo "  make up          - Start all services"
	@echo "  make down        - Stop all services"
	@echo "  make restart     - Restart all services"
	@echo "  make rebuild     - Rebuild and restart services"
	@echo "  make logs        - View logs (all services)"
	@echo "  make ps          - Show service status"
	@echo "  make stats       - Show resource usage"
	@echo "  make clean       - Remove containers and images"
	@echo "  make clean-all   - Remove containers, images, and volumes"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build-all      - Build all binaries (web, mcp, setup)"
	@echo "  make build-backup   - Build backup service binary"
	@echo "  make build-setup    - Build the setup wizard binary"
	@echo "  make vendor-assets  - Download vendor assets for Web UI"
	@echo ""
	@echo "Testing Commands:"
	@echo "  make test                - Run all tests"
	@echo "  make test-unit           - Run unit tests only"
	@echo "  make test-integration    - Run integration tests"
	@echo "  make test-integration-v  - Run integration tests (verbose)"
	@echo "  make test-bench          - Run benchmarks"
	@echo "  make test-load           - Run load tests"
	@echo "  make test-coverage       - Run tests with coverage"
	@echo "  make test-golden-update  - Update golden files"
	@echo ""
	@echo "Service-Specific Logs:"
	@echo "  make logs-app    - View memento app logs"
	@echo "  make logs-ollama - View ollama logs"
	@echo "  make logs-backup - View backup service logs"
	@echo ""
	@echo "Data Management:"
	@echo "  make backup         - Trigger manual backup"
	@echo "  make backup-list    - List all backups"
	@echo "  make backup-health  - Check backup service health"
	@echo "  make db-shell       - Open SQLite shell"
	@echo "  make export-db      - Export database"
	@echo ""
	@echo "Maintenance:"
	@echo "  make health     - Check service health"
	@echo "  make pull-model - Pull latest LLM model (qwen2.5:7b)"
	@echo "  make setup      - Run the interactive setup wizard"

# Build Docker images
build:
	docker compose build

# Build backup service binary (local)
build-backup:
	@echo "Building memento-backup..."
	@mkdir -p bin
	go build -o bin/memento-backup ./cmd/memento-backup
	@echo "Built: bin/memento-backup"

# Run the interactive setup wizard
setup: ## Run the interactive setup wizard
	go run ./cmd/memento-setup/

# Build the setup binary
build-setup: ## Build the setup binary
	go build -o memento-setup ./cmd/memento-setup/

# Build all binaries
build-all: ## Build all binaries
	go build -o memento-web ./cmd/memento-web/
	go build -o memento-mcp ./cmd/memento-mcp/
	go build -o memento-setup ./cmd/memento-setup/

# Download vendor assets for Web UI
.PHONY: vendor-assets
vendor-assets:
	@echo "Downloading vendor assets..."
	@./scripts/download-vendor-assets.sh

# Start services
up:
	docker compose up -d

# Stop services
down:
	docker compose down

# Restart services
restart:
	docker compose restart

# View logs (all services)
logs:
	docker compose logs -f

# View app logs
logs-app:
	docker compose logs -f memento

# View ollama logs
logs-ollama:
	docker compose logs -f ollama

# View backup logs
logs-backup:
	docker compose logs -f backup

# Show service status
ps:
	docker compose ps

# Clean up (remove containers and images)
clean:
	docker compose down
	docker rmi memento-go-memento memento-go-backup 2>/dev/null || true

# Clean up everything including volumes
clean-all: clean
	docker volume rm memento-data memento-backups ollama-models 2>/dev/null || true

# Run all tests
test:
	@echo "Running all tests..."
	go test -v ./tests/...

# Run unit tests only (fast tests in tests/ root)
test-unit:
	@echo "Running unit tests..."
	go test -v -short ./tests/*.go

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@chmod +x ./scripts/run-integration-tests.sh
	./scripts/run-integration-tests.sh

# Run integration tests (verbose)
test-integration-v:
	@echo "Running integration tests (verbose)..."
	@chmod +x ./scripts/run-integration-tests.sh
	./scripts/run-integration-tests.sh -v

# Run benchmarks
test-bench:
	@echo "Running benchmarks..."
	@chmod +x ./scripts/run-benchmarks.sh
	./scripts/run-benchmarks.sh

# Run load tests
test-load:
	@echo "Running load tests..."
	@chmod +x ./scripts/run-load-tests.sh
	./scripts/run-load-tests.sh

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Update golden files
test-golden-update:
	@echo "Updating golden files..."
	go test -v ./tests/integration/ -update

# Trigger manual backup
backup:
	docker compose exec backup /app/memento-backup --oneshot

# List all backups
backup-list:
	docker compose exec backup /app/memento-backup --list

# Check backup service health
backup-health:
	docker compose exec backup /app/memento-backup --health

# Open database shell
db-shell:
	docker compose exec memento sqlite3 /data/memento.db

# Health check
health:
	@echo "Checking memento health..."
	@docker compose exec memento wget -q -O- http://localhost:6363/health || echo "Health check failed"
	@echo ""
	@echo "Checking ollama health..."
	@docker compose exec ollama ollama list || echo "Ollama not ready"

# Pull latest model
pull-model:
	docker compose exec ollama ollama pull qwen2.5:7b

# Rebuild and restart (useful after code changes)
rebuild:
	docker compose up -d --build

# View resource usage
stats:
	docker stats --no-stream

# Export database
export-db:
	@mkdir -p ./exports
	docker compose cp memento:/data/memento.db ./exports/memento-export-$(shell date +%Y%m%d-%H%M%S).db
	@echo "Database exported to ./exports/"

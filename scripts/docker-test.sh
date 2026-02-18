#!/bin/bash
# Docker deployment test script
# Verifies that all services start correctly and are healthy

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Check if Docker is running
log_step "Checking Docker installation..."
if ! command -v docker &> /dev/null; then
    log_error "Docker is not installed"
    exit 1
fi

if ! docker info &> /dev/null; then
    log_error "Docker daemon is not running"
    exit 1
fi

log_info "Docker is installed and running"

# Check if docker compose is available
log_step "Checking Docker Compose..."
if ! docker compose version &> /dev/null; then
    log_error "Docker Compose is not available"
    exit 1
fi

log_info "Docker Compose is available"

# Stop any existing services
log_step "Stopping existing services (if any)..."
docker compose down &> /dev/null || true
log_info "Cleaned up existing services"

# Build images
log_step "Building Docker images..."
if docker compose build; then
    log_info "Images built successfully"
else
    log_error "Failed to build images"
    exit 1
fi

# Start services
log_step "Starting services..."
if docker compose up -d; then
    log_info "Services started"
else
    log_error "Failed to start services"
    exit 1
fi

# Wait for services to be ready
log_step "Waiting for services to be healthy..."
sleep 5

# Check service status
log_step "Checking service status..."
docker compose ps

# Check memento health
log_step "Checking Memento application health..."
max_retries=30
retry_count=0

while [ $retry_count -lt $max_retries ]; do
    if docker compose exec -T memento wget -q -O- http://localhost:6363/health &> /dev/null; then
        log_info "Memento is healthy"
        break
    fi

    retry_count=$((retry_count + 1))
    if [ $retry_count -eq $max_retries ]; then
        log_error "Memento health check timed out"
        log_info "Viewing logs..."
        docker compose logs memento
        exit 1
    fi

    sleep 2
done

# Check Ollama
log_step "Checking Ollama service..."
if docker compose exec -T ollama ollama list &> /dev/null; then
    log_info "Ollama is running"
else
    log_warn "Ollama might not be fully ready yet"
fi

# Check backup service
log_step "Checking backup service..."
if docker compose ps backup | grep -q "Up"; then
    log_info "Backup service is running"
else
    log_error "Backup service is not running"
fi

# Check volumes
log_step "Checking volumes..."
for volume in memento-data ollama-models memento-backups; do
    if docker volume inspect $volume &> /dev/null; then
        log_info "Volume '$volume' exists"
    else
        log_error "Volume '$volume' not found"
    fi
done

# Summary
echo ""
echo "========================================"
log_info "Docker deployment test completed successfully!"
echo "========================================"
echo ""
echo "Services running:"
echo "  - Memento app:    http://localhost:6363"
echo "  - Ollama:         http://localhost:11434"
echo "  - Backup service: Running in background"
echo ""
echo "Useful commands:"
echo "  docker compose logs -f        # View all logs"
echo "  docker compose ps             # Service status"
echo "  docker compose down           # Stop services"
echo "  make help                     # See all commands"
echo ""

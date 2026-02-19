#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# Memento — Interactive Launch Script
# https://github.com/scrypster/memento
# ============================================================================

# Colors
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
DIM='\033[2m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
fail()  { echo -e "${RED}[✗]${NC} $1"; }
step()  { echo -e "${CYAN}[→]${NC} $1"; }
header(){ echo -e "\n${BOLD}$1${NC}"; }

# ============================================================================
# Banner
# ============================================================================
echo -e "${BOLD}"
echo "  __  __                           _        "
echo " |  \/  | ___ _ __ ___   ___ _ __ | |_ ___  "
echo " | |\/| |/ _ \ '_ \` _ \ / _ \ '_ \| __/ _ \ "
echo " | |  | |  __/ | | | | |  __/ | | | || (_) |"
echo " |_|  |_|\___|_| |_| |_|\___|_| |_|\__\___/ "
echo ""
echo -e "${NC}${DIM}  Persistent memory for AI tools${NC}"
echo ""

# ============================================================================
# Detect OS
# ============================================================================
OS="$(uname -s)"
case "$OS" in
    Linux*)  PLATFORM="linux";;
    Darwin*) PLATFORM="mac";;
    *)       PLATFORM="other";;
esac

# ============================================================================
# Choose setup mode
# ============================================================================
header "How would you like to run Memento?"
echo ""
echo "  ${BOLD}1)${NC} Docker ${DIM}(recommended — zero dependencies)${NC}"
echo "  ${BOLD}2)${NC} Local  ${DIM}(build from source with Go)${NC}"
echo ""

while true; do
    printf "  Select [1/2]: "
    read -r choice
    case "$choice" in
        1) MODE="docker"; break;;
        2) MODE="local"; break;;
        *) echo "  Please enter 1 or 2.";;
    esac
done

# ============================================================================
# Docker Setup
# ============================================================================
if [ "$MODE" = "docker" ]; then
    header "Docker Setup"
    echo ""

    # --- Check Docker ---
    step "Checking for Docker..."
    if ! command -v docker &>/dev/null; then
        fail "Docker is not installed."
        echo ""
        echo "  Install Docker Desktop:"
        if [ "$PLATFORM" = "mac" ]; then
            echo "    https://docs.docker.com/desktop/install/mac-install/"
        elif [ "$PLATFORM" = "linux" ]; then
            echo "    https://docs.docker.com/engine/install/"
        else
            echo "    https://docs.docker.com/get-docker/"
        fi
        exit 1
    fi
    info "Docker found: $(docker --version | head -1)"

    # --- Check Docker daemon ---
    step "Checking Docker daemon..."
    if ! docker info &>/dev/null; then
        fail "Docker daemon is not running. Please start Docker Desktop and try again."
        exit 1
    fi
    info "Docker daemon is running"

    # --- Check Docker Compose ---
    step "Checking for Docker Compose..."
    if docker compose version &>/dev/null; then
        COMPOSE_CMD="docker compose"
        info "Docker Compose found: $(docker compose version --short 2>/dev/null || echo 'v2+')"
    elif command -v docker-compose &>/dev/null; then
        COMPOSE_CMD="docker-compose"
        info "Docker Compose found: $(docker-compose --version | head -1)"
    else
        fail "Docker Compose is not installed."
        echo "  Install it via Docker Desktop or: https://docs.docker.com/compose/install/"
        exit 1
    fi

    # --- Check disk space ---
    step "Checking available disk space..."
    if [ "$PLATFORM" = "mac" ]; then
        AVAIL_GB=$(df -g / | tail -1 | awk '{print $4}')
    else
        AVAIL_GB=$(df -BG / | tail -1 | awk '{print $4}' | tr -d 'G')
    fi
    if [ "$AVAIL_GB" -lt 8 ] 2>/dev/null; then
        warn "Low disk space (${AVAIL_GB}GB free). Ollama models need ~5GB."
        echo "    Continue anyway? [y/N] "
        read -r cont
        if [ "$cont" != "y" ] && [ "$cont" != "Y" ]; then
            exit 1
        fi
    else
        info "Disk space OK (${AVAIL_GB}GB available)"
    fi

    # --- Launch ---
    header "Launching Memento..."
    echo ""
    step "Building and starting containers..."
    echo -e "  ${DIM}This will pull/build images on first run (~2 min).${NC}"
    echo -e "  ${DIM}Ollama models download in the background (~5 GB on first run).${NC}"
    echo ""

    $COMPOSE_CMD up -d --build

    echo ""
    info "Containers are starting!"
    echo ""

    # --- Wait for health ---
    step "Waiting for Memento to be ready..."
    READY=0
    for i in $(seq 1 30); do
        if curl -sf "http://localhost:6363/api/health" > /dev/null 2>&1; then
            READY=1
            break
        fi
        printf "."
        sleep 2
    done
    echo ""

    if [ "$READY" = "1" ]; then
        info "Memento is running!"
    else
        warn "Memento is still starting (Ollama models may be downloading)."
        echo "    Check progress with: docker compose logs -f memento"
    fi

    echo ""
    header "You're all set!"
    echo ""
    echo -e "  ${BOLD}Web UI:${NC}           http://localhost:6363"
    echo -e "  ${BOLD}View logs:${NC}        $COMPOSE_CMD logs -f memento"
    echo -e "  ${BOLD}Stop:${NC}             $COMPOSE_CMD down"
    echo -e "  ${BOLD}Restart:${NC}          $COMPOSE_CMD restart"
    echo ""
    echo -e "  ${DIM}Next: Open the web UI and go to the Integrations page${NC}"
    echo -e "  ${DIM}to connect Claude Code, Cursor, Windsurf, or any MCP client.${NC}"
    echo ""

    exit 0
fi

# ============================================================================
# Local Setup
# ============================================================================
header "Local Setup"
echo ""
CHECKS_PASSED=0
CHECKS_TOTAL=0

# --- Check Go ---
CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
step "Checking for Go..."
if ! command -v go &>/dev/null; then
    fail "Go is not installed."
    echo "    Install Go 1.23+: https://go.dev/dl/"
else
    GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1)
    GO_MAJOR=$(echo "$GO_VERSION" | grep -oE '[0-9]+\.[0-9]+')
    info "Go found: $GO_VERSION"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
fi

# --- Check Ollama ---
CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
step "Checking for Ollama..."
if ! command -v ollama &>/dev/null; then
    fail "Ollama is not installed."
    if [ "$PLATFORM" = "mac" ]; then
        echo "    Install: brew install ollama"
        echo "    Or visit: https://ollama.com/download/mac"
    elif [ "$PLATFORM" = "linux" ]; then
        echo "    Install: curl -fsSL https://ollama.com/install.sh | sh"
    else
        echo "    Install: https://ollama.com/download"
    fi
else
    info "Ollama found: $(ollama --version 2>/dev/null || echo 'installed')"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
fi

# --- Check Ollama is running ---
CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
step "Checking if Ollama is running..."
OLLAMA_URL="${MEMENTO_OLLAMA_URL:-http://localhost:11434}"
if curl -sf "$OLLAMA_URL/api/tags" > /dev/null 2>&1; then
    info "Ollama is running at $OLLAMA_URL"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))

    # --- Check required models ---
    EXTRACTION_MODEL="${MEMENTO_OLLAMA_MODEL:-qwen2.5:7b}"
    EMBEDDING_MODEL="${MEMENTO_EMBEDDING_MODEL:-nomic-embed-text}"

    CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
    step "Checking for extraction model: $EXTRACTION_MODEL"
    if curl -sf "$OLLAMA_URL/api/tags" | grep -q "$EXTRACTION_MODEL"; then
        info "Model $EXTRACTION_MODEL is available"
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    else
        warn "Model $EXTRACTION_MODEL not found. Pulling now..."
        if ollama pull "$EXTRACTION_MODEL"; then
            info "Model $EXTRACTION_MODEL pulled successfully"
            CHECKS_PASSED=$((CHECKS_PASSED + 1))
        else
            fail "Failed to pull $EXTRACTION_MODEL"
        fi
    fi

    CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
    step "Checking for embedding model: $EMBEDDING_MODEL"
    if curl -sf "$OLLAMA_URL/api/tags" | grep -q "$EMBEDDING_MODEL"; then
        info "Model $EMBEDDING_MODEL is available"
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    else
        warn "Model $EMBEDDING_MODEL not found. Pulling now..."
        if ollama pull "$EMBEDDING_MODEL"; then
            info "Model $EMBEDDING_MODEL pulled successfully"
            CHECKS_PASSED=$((CHECKS_PASSED + 1))
        else
            fail "Failed to pull $EMBEDDING_MODEL"
        fi
    fi
else
    fail "Ollama is not running."
    if command -v ollama &>/dev/null; then
        echo "    Start it with: ollama serve"
    fi
fi

# --- Summary ---
echo ""
header "Preflight Check: $CHECKS_PASSED/$CHECKS_TOTAL passed"

if [ "$CHECKS_PASSED" -lt 3 ]; then
    echo ""
    fail "Required dependencies are missing. Please install them and try again."
    exit 1
fi

# --- Check Node.js ---
CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
step "Checking for Node.js..."
if ! command -v node &>/dev/null; then
    fail "Node.js is not installed (needed for CSS build)."
    echo "    Install Node.js 18+: https://nodejs.org/"
else
    NODE_VERSION=$(node --version)
    info "Node.js found: $NODE_VERSION"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
fi

# --- Build ---
echo ""
header "Building Memento..."
echo ""

step "Downloading vendor JS libraries..."
chmod +x ./scripts/download-vendor-assets.sh
./scripts/download-vendor-assets.sh
info "Vendor assets downloaded"

step "Building CSS (Vite + Tailwind)..."
npm install --no-audit --no-fund
npx vite build
info "CSS built"

step "Building memento-web (dashboard + API)..."
go build -o memento-web ./cmd/memento-web/
info "memento-web built"

step "Building memento-mcp (MCP server for AI tools)..."
go build -o memento-mcp ./cmd/memento-mcp/
info "memento-mcp built"

step "Building memento-setup (setup wizard)..."
go build -o memento-setup ./cmd/memento-setup/
info "memento-setup built"

# --- Create data directory ---
mkdir -p data

# --- Start ---
echo ""
header "Ready to launch!"
echo ""
echo -e "  ${BOLD}Start the web dashboard:${NC}"
echo "    ./memento-web"
echo ""
echo -e "  ${BOLD}Run the interactive setup wizard:${NC}"
echo "    ./memento-setup"
echo ""
echo -e "  ${BOLD}Register MCP with Claude Code:${NC}"
echo "    claude mcp add memento -- $(pwd)/memento-mcp -e MEMENTO_DATA_PATH=$(pwd)/data"
echo ""
echo -e "  ${DIM}The web UI at http://localhost:6363 has copy-paste configs${NC}"
echo -e "  ${DIM}for Claude Desktop, Cursor, Windsurf, and other MCP clients.${NC}"
echo ""

# --- Offer to start ---
printf "  Start the web dashboard now? [Y/n]: "
read -r start
if [ "$start" != "n" ] && [ "$start" != "N" ]; then
    echo ""
    info "Starting Memento at http://localhost:6363 ..."
    echo -e "  ${DIM}Press Ctrl+C to stop.${NC}"
    echo ""
    ./memento-web
fi

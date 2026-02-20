#!/usr/bin/env bash
set -e

# Memento installer
# Usage: curl -fsSL https://raw.githubusercontent.com/scrypster/memento/main/install.sh | sh

REPO="scrypster/memento"
INSTALL_DIR="${MEMENTO_INSTALL_DIR:-$HOME/.local/bin}"
DATA_DIR="${MEMENTO_DATA_DIR:-$HOME/.memento}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo "  ███╗   ███╗███████╗███╗   ███╗███████╗███╗   ██╗████████╗ ██████╗ "
echo "  ████╗ ████║██╔════╝████╗ ████║██╔════╝████╗  ██║╚══██╔══╝██╔═══██╗"
echo "  ██╔████╔██║█████╗  ██╔████╔██║█████╗  ██╔██╗ ██║   ██║   ██║   ██║"
echo "  ██║╚██╔╝██║██╔══╝  ██║╚██╔╝██║██╔══╝  ██║╚██╗██║   ██║   ██║   ██║"
echo "  ██║ ╚═╝ ██║███████╗██║ ╚═╝ ██║███████╗██║ ╚████║   ██║   ╚██████╔╝"
echo "  ╚═╝     ╚═╝╚══════╝╚═╝     ╚═╝╚══════╝╚═╝  ╚═══╝   ╚═╝    ╚═════╝ "
echo ""
echo "  Persistent Memory for AI"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

echo -e "${YELLOW}Detecting environment...${NC}"

# Check for Docker
if command -v docker &>/dev/null && command -v docker-compose &>/dev/null || \
   (command -v docker &>/dev/null && docker compose version &>/dev/null 2>&1); then
    echo -e "${GREEN}Docker detected. Recommended: use Docker Compose for the full stack.${NC}"
    echo ""
    echo "  Quick start with Docker:"
    echo "  $ git clone https://github.com/$REPO"
    echo "  $ cd memento"
    echo "  $ docker compose up -d"
    echo "  $ open http://localhost:6363"
    echo ""
fi

# Check for Go
if command -v go &>/dev/null; then
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo -e "${GREEN}Go $GO_VERSION detected.${NC}"

    echo -e "${YELLOW}Installing memento-setup...${NC}"
    go install github.com/scrypster/memento/cmd/memento-setup@latest

    echo ""
    echo -e "${GREEN}Installation complete!${NC}"
    echo ""
    echo "  Run the setup wizard:"
    echo "  $ memento-setup"
    echo ""
else
    echo -e "${YELLOW}Go not found. Installing via Docker...${NC}"

    if ! command -v docker &>/dev/null; then
        echo -e "${RED}Neither Go nor Docker found. Please install one:${NC}"
        echo "  Docker: https://docs.docker.com/get-docker/"
        echo "  Go:     https://go.dev/dl/"
        exit 1
    fi

    echo ""
    echo "  Start with Docker Compose:"
    echo "  $ git clone https://github.com/$REPO"
    echo "  $ cd memento"
    echo "  $ docker compose up -d"
    echo "  $ open http://localhost:6363"
    echo ""
fi

echo -e "${GREEN}Memento is ready to give your AI a persistent memory.${NC}"
echo ""
echo "  Docs:    https://github.com/$REPO"
echo "  Issues:  https://github.com/$REPO/issues"
echo ""

#!/bin/sh
set -e

OLLAMA_URL="${MEMENTO_OLLAMA_URL:-http://ollama:11434}"

# Wait for Ollama to be ready (max 120s)
echo "Waiting for Ollama at ${OLLAMA_URL}..."
OLLAMA_READY=0
for i in $(seq 1 24); do
    if curl -sf "${OLLAMA_URL}/api/tags" > /dev/null 2>&1; then
        OLLAMA_READY=1
        echo "Ollama is ready"
        break
    fi
    echo "   Waiting... ($i/24)"
    sleep 5
done

# pull_model waits for the Ollama /api/pull streaming response to complete.
# Ollama returns newline-delimited JSON; we read until we see "success" or EOF.
pull_model() {
    MODEL_NAME="$1"
    LABEL="$2"

    # Check if already present
    if curl -sf "${OLLAMA_URL}/api/tags" 2>/dev/null | grep -q "\"${MODEL_NAME}\""; then
        echo "${LABEL}: ${MODEL_NAME} already installed"
        return 0
    fi

    echo "${LABEL}: pulling ${MODEL_NAME} (this may take several minutes)..."

    # Stream the pull — print periodic progress, wait for completion
    TMPFILE=$(mktemp)
    HTTP_CODE=$(curl -s -o "$TMPFILE" -w "%{http_code}" -X POST "${OLLAMA_URL}/api/pull" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"${MODEL_NAME}\",\"stream\":false}")

    if [ "$HTTP_CODE" = "200" ]; then
        echo "${LABEL}: ${MODEL_NAME} ready"
    else
        echo "${LABEL}: could not pull ${MODEL_NAME} (HTTP ${HTTP_CODE}), will retry on next start"
    fi
    rm -f "$TMPFILE"
}

if [ "$OLLAMA_READY" = "1" ]; then
    EXTRACTION_MODEL="${MEMENTO_OLLAMA_MODEL:-qwen2.5:7b}"
    EMBEDDING_MODEL="${MEMENTO_EMBEDDING_MODEL:-nomic-embed-text}"

    pull_model "$EXTRACTION_MODEL" "Extraction model"
    pull_model "$EMBEDDING_MODEL" "Embedding model"
else
    echo "WARNING: Ollama not available after 120s"
    echo "  Memento will start but enrichment (entity extraction, embeddings)"
    echo "  will not work until Ollama is running with the required models."
    echo ""
    echo "  Required models:"
    echo "    - ${MEMENTO_OLLAMA_MODEL:-qwen2.5:7b} (entity extraction)"
    echo "    - ${MEMENTO_EMBEDDING_MODEL:-nomic-embed-text} (embeddings)"
fi

# Initialize default connections.json (always write to ensure correct schema)
CONFIG_FILE="${MEMENTO_CONNECTIONS_CONFIG:-/app/config/connections.json}"
echo "Writing default connections.json..."
mkdir -p "$(dirname "$CONFIG_FILE")"
cat > "$CONFIG_FILE" << 'JSONEOF'
{
  "default_connection": "default",
  "connections": [
    {
      "name": "default",
      "display_name": "Default",
      "description": "Default memory workspace",
      "enabled": true,
      "database": {
        "type": "sqlite",
        "path": "/data/memento.db"
      },
      "llm": {
        "provider": "ollama",
        "model": "qwen2.5:7b"
      }
    }
  ],
  "settings": {
    "auto_create_default": true,
    "max_connections": 10,
    "allow_user_create": true
  }
}
JSONEOF

echo "Starting Memento..."
exec ./memento-web "$@"

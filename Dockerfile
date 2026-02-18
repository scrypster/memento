# ---- Build stage ----
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o memento-web ./cmd/memento-web/
RUN go build -o memento-mcp ./cmd/memento-mcp/
RUN go build -o memento-setup ./cmd/memento-setup/

# ---- Runtime stage ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget curl sqlite

WORKDIR /app
COPY --from=builder /app/memento-web .
COPY --from=builder /app/memento-mcp .
COPY --from=builder /app/memento-setup .
COPY web/ web/
COPY config/ config/
COPY scripts/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Create data directory
RUN mkdir -p /data /app/config

# Default environment
ENV MEMENTO_PORT=6363
ENV MEMENTO_HOST=0.0.0.0
ENV MEMENTO_DATA_PATH=/data
ENV MEMENTO_STORAGE_ENGINE=sqlite

EXPOSE 6363

CMD ["/app/docker-entrypoint.sh"]

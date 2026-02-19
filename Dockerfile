# ---- CSS build stage ----
FROM node:20-alpine AS css-builder
WORKDIR /app
COPY package.json ./
RUN npm install --no-audit --no-fund
COPY web/static/css/ web/static/css/
COPY web/templates/ web/templates/
COPY web/static/js/ web/static/js/
COPY tailwind.config.js postcss.config.js vite.config.js ./
RUN npx vite build

# ---- Vendor JS download stage ----
FROM alpine:3.19 AS vendor-builder
RUN apk add --no-cache curl bash
WORKDIR /app
COPY scripts/download-vendor-assets.sh scripts/
RUN chmod +x scripts/download-vendor-assets.sh && ./scripts/download-vendor-assets.sh

# ---- Go build stage ----
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
COPY --from=css-builder /app/web/static/dist/ web/static/dist/
COPY --from=vendor-builder /app/web/static/vendor/ web/static/vendor/
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

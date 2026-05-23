# ============================================================
# Stage 1: Build frontend
# ============================================================
FROM node:24-alpine AS frontend

WORKDIR /app/web

# Layer cache: install deps first (only changes when package files change)
COPY web/package.json web/package-lock.json ./
RUN npm install

COPY web/ ./
RUN npm run build

# ============================================================
# Stage 2: Build Go binary
# ============================================================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Layer cache: download modules first (only changes when go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy frontend build output into the embedded FS path
COPY --from=frontend /app/web/dist ./web/dist

# Build static binary with version info injected via ldflags
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o /sqlflow ./cmd/server/

# ============================================================
# Stage 3: Runtime (minimal image)
# ============================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user before copying files
RUN addgroup -S sqlflow && adduser -S -G sqlflow sqlflow

WORKDIR /app

# Copy with ownership in a single layer (avoids chown duplication)
COPY --chown=sqlflow:sqlflow \
    --from=builder /sqlflow /app/sqlflow
COPY --chown=sqlflow:sqlflow config/config.yaml /app/config.yaml
COPY --chown=sqlflow:sqlflow entrypoint.sh /app/entrypoint.sh
COPY --chown=sqlflow:sqlflow --from=frontend /app/web/dist /app/web/dist

RUN chmod +x /app/entrypoint.sh && mkdir -p /app/data && chown sqlflow:sqlflow /app/data

USER sqlflow

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/api/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]

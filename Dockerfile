# =========================
# Stage 1: Build
# =========================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Required for downloading Go dependencies
RUN apk add --no-cache ca-certificates git

# Copy dependency files first.
# This allows Docker/Podman to cache the dependency layer.
COPY go.mod go.sum ./

RUN go mod download

# Copy application source
COPY . .

# Build the API
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /app/bin/api ./cmd/api

# Build the worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /app/bin/worker ./cmd/worker


# =========================
# Stage 2: Runtime
# =========================
FROM alpine:3.22 AS runtime

WORKDIR /app

# TLS certificates are needed for HTTPS/API calls.
RUN apk add --no-cache ca-certificates

# Copy both compiled applications
COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/bin/worker /app/worker

# Database migrations required at API startup
COPY --from=builder /app/internal/database/migrations /app/internal/database/migrations

# API port
EXPOSE 8081

# Default command.
# Compose will override this for the worker.
CMD ["./api"]
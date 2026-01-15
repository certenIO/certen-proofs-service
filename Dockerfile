# Certen Proof Artifact Service
# Multi-stage build for minimal production image

# Stage 1: Build the Go binary
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o proof-service ./cmd/proof-service

# Stage 2: Create minimal production image
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/proof-service .

# Copy migrations for database setup
COPY --from=builder /app/pkg/database/migrations ./migrations

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

# Expose API port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the service
ENTRYPOINT ["./proof-service"]

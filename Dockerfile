# Multi-stage build for both server and client
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy source code
COPY . .

# Build binaries with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o bin/holdem-server ./cmd/holdem-server && \
    go build -o bin/holdem-client ./cmd/holdem-client && \
    go build -o bin/holdem ./cmd/holdem

# Final stage - minimal alpine image
FROM alpine:3.21.0

# Install ca-certificates for HTTPS requests and terminal utilities
RUN apk --no-cache add ca-certificates bash curl

WORKDIR /app

# Copy binaries from builder stage
COPY --from=builder /app/bin/ ./bin/

# Create directories for logs and hand history
RUN mkdir -p logs handhistory

# Expose server port
EXPOSE 8080

# Default command (can be overridden in docker-compose)
CMD ["./bin/holdem-server"]

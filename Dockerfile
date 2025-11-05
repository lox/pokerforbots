# Build stage
FROM golang:1.25-alpine AS builder

ARG VERSION=dev

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary with version info
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o pokerforbots ./cmd/pokerforbots

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/pokerforbots /app/pokerforbots

# Copy docs for reference
COPY --from=builder /build/docs /app/docs
COPY --from=builder /build/README.md /app/README.md

# Create non-root user
RUN addgroup -g 1000 poker && \
    adduser -D -u 1000 -G poker poker && \
    chown -R poker:poker /app

USER poker

# Expose default port
EXPOSE 8080

# Default command runs server
ENTRYPOINT ["/app/pokerforbots"]
CMD ["server"]

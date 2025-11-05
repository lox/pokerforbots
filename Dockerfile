# Runtime stage - GoReleaser provides pre-built binary
FROM alpine:latest

ARG TARGETPLATFORM

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy pre-built binary from GoReleaser context
# GoReleaser places binaries in $TARGETPLATFORM/ subdirectory
COPY ${TARGETPLATFORM}/pokerforbots /app/pokerforbots

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

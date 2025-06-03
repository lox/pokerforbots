#!/bin/bash

# Docker testing script for Holdem CLI

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

# Check if Docker and docker-compose are available
check_dependencies() {
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed or not in PATH"
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null; then
        error "docker-compose is not installed or not in PATH"
        exit 1
    fi
}

# Build the Docker image
build() {
    log "Building Docker image..."
    docker-compose build
}

# Start the server only
start_server() {
    log "Starting Holdem server..."
    docker-compose up -d holdem-server

    log "Waiting for server to be healthy..."
    timeout 30 bash -c 'until docker-compose exec holdem-server curl -f http://localhost:8080/health; do sleep 2; done'

    if [ $? -eq 0 ]; then
        log "Server is running and healthy at http://localhost:8080"
        log "You can check server logs with: docker-compose logs -f holdem-server"
    else
        error "Server failed to start or become healthy within 30 seconds"
        exit 1
    fi
}

# Start server and automated clients
start_full() {
    log "Starting full setup (server + automated clients)..."
    docker-compose up -d holdem-server holdem-client-1 holdem-client-2

    log "Waiting for server to be healthy..."
    timeout 30 bash -c 'until docker-compose exec holdem-server curl -f http://localhost:8080/health; do sleep 2; done'

    if [ $? -eq 0 ]; then
        log "Full setup is running!"
        log "Server: http://localhost:8080"
        log "Check logs with: docker-compose logs -f"
        log "Stop with: docker-compose down"
    else
        error "Setup failed to start properly"
        exit 1
    fi
}

# Connect an interactive client
connect_client() {
    local player_name=${1:-"Player$(date +%s)"}
    log "Connecting interactive client as '$player_name'..."
    docker-compose run --rm holdem-client-interactive ./bin/holdem-client --server=http://holdem-server:8080 --player="$player_name"
}

# Show logs
logs() {
    docker-compose logs -f "$@"
}

# Stop all services
stop() {
    log "Stopping all services..."
    docker-compose down
}

# Clean up everything
clean() {
    log "Cleaning up Docker resources..."
    docker-compose down -v --rmi local
    docker system prune -f
}

# Show status
status() {
    log "Service status:"
    docker-compose ps

    echo ""
    log "Server health check:"
    if docker-compose exec holdem-server curl -f http://localhost:8080/health 2>/dev/null; then
        log "Server is healthy ✓"
    else
        warn "Server is not responding ✗"
    fi
}

# Test the complete flow
test_flow() {
    log "Running complete test flow..."

    # Clean start
    stop

    # Build and start server
    build
    start_server

    # Show some status
    status

    log "Test completed! You can now:"
    log "  1. Connect a client: $0 client [player_name]"
    log "  2. Start automated clients: $0 full"
    log "  3. View logs: $0 logs"
    log "  4. Stop everything: $0 stop"
}

# Show usage
usage() {
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  build           Build the Docker image"
    echo "  server          Start server only"
    echo "  full            Start server + automated clients"
    echo "  client [name]   Connect interactive client (optional player name)"
    echo "  logs [service]  Show logs (optional service filter)"
    echo "  status          Show service status"
    echo "  test            Run complete test flow"
    echo "  stop            Stop all services"
    echo "  clean           Clean up all Docker resources"
    echo ""
    echo "Examples:"
    echo "  $0 test                    # Run complete test"
    echo "  $0 server                  # Start server only"
    echo "  $0 client Alice            # Connect as 'Alice'"
    echo "  $0 logs holdem-server      # Show server logs"
    echo "  $0 full                    # Start everything"
}

# Create logs directory if it doesn't exist
mkdir -p logs handhistory

# Main command dispatcher
case "${1:-}" in
    build)
        check_dependencies
        build
        ;;
    server)
        check_dependencies
        start_server
        ;;
    full)
        check_dependencies
        start_full
        ;;
    client)
        check_dependencies
        connect_client "$2"
        ;;
    logs)
        logs "$2"
        ;;
    status)
        status
        ;;
    test)
        check_dependencies
        test_flow
        ;;
    stop)
        stop
        ;;
    clean)
        clean
        ;;
    help|--help|-h)
        usage
        ;;
    "")
        warn "No command specified"
        usage
        exit 1
        ;;
    *)
        error "Unknown command: $1"
        usage
        exit 1
        ;;
esac

#!/bin/bash

# Demo script for PokerForBots
# Shows the server running with multiple bot strategies playing against each other

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== PokerForBots Demo ===${NC}"
echo ""

# Check if server is already running
if lsof -i :8080 > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port 8080...${NC}"
    lsof -i :8080 | grep LISTEN | awk '{print $2}' | xargs kill -9 2>/dev/null || true
    sleep 1
fi

# Build the server and bot
echo -e "${GREEN}Building server and bot...${NC}"
mkdir -p dist
go build -o dist/server cmd/server/main.go
go build -o dist/testbot cmd/testbot/main.go

# Start the server
echo -e "${GREEN}Starting server on port 8080...${NC}"
./dist/server &
SERVER_PID=$!
sleep 2

# Function to cleanup on exit
cleanup() {
    echo ""
    echo -e "${YELLOW}Shutting down...${NC}"

    # Kill bots
    pkill -f "dist/testbot" 2>/dev/null || true

    # Kill server
    kill $SERVER_PID 2>/dev/null || true

    echo -e "${GREEN}Demo stopped.${NC}"
    exit 0
}

trap cleanup INT TERM

# Check server is running
if ! curl -s http://localhost:8080/stats > /dev/null 2>&1; then
    echo -e "${RED}Server failed to start!${NC}"
    exit 1
fi

echo -e "${GREEN}Server is running!${NC}"
echo ""

# Launch bots with different strategies
echo -e "${GREEN}Launching bots...${NC}"

# Start 2 calling stations
echo "  - Starting 2 calling station bots..."
./dist/testbot -strategy calling-station -count 2 > logs/calling-station.log 2>&1 &

# Start 2 random bots
echo "  - Starting 2 random bots..."
./dist/testbot -strategy random -count 2 > logs/random.log 2>&1 &

# Start 2 aggressive bots
echo "  - Starting 2 aggressive bots..."
./dist/testbot -strategy aggressive -count 2 > logs/aggressive.log 2>&1 &

sleep 2

echo ""
echo -e "${GREEN}Demo is running with 6 bots playing poker!${NC}"
echo ""
echo "Bot strategies:"
echo "  - 2x Calling Station (always calls/checks)"
echo "  - 2x Random (random valid actions)"
echo "  - 2x Aggressive (raises frequently)"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop the demo${NC}"
echo ""

# Create logs directory if it doesn't exist
mkdir -p logs

# Monitor and display stats
while true; do
    # Get stats from server
    STATS=$(curl -s http://localhost:8080/stats 2>/dev/null || echo "Server not responding")

    # Clear line and print stats
    echo -ne "\r${GREEN}Server stats:${NC} $STATS"

    sleep 1
done
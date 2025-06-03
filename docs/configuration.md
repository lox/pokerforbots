# HCL Configuration Guide

The Holdem CLI uses HCL (HashiCorp Configuration Language) for flexible, human-readable configuration files.

## Server Configuration (`holdem-server.hcl`)

### Basic Structure

```hcl
server {
  address   = "localhost"
  port      = 8080
  log_level = "info"
  log_file  = "holdem-server.log"
}

table "table_name" {
  max_players = 6
  small_blind = 1
  big_blind   = 2
  buy_in_min  = 100
  buy_in_max  = 1000
  auto_start  = true
}
```

### Server Block

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `address` | string | "localhost" | Server bind address |
| `port` | number | 8080 | Server port |
| `log_level` | string | "info" | Log level: debug, info, warn, error |
| `log_file` | string | "holdem-server.log" | Log file path |

### Table Block

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max_players` | number | 6 | Maximum players (2-10) |
| `small_blind` | number | **required** | Small blind amount |
| `big_blind` | number | **required** | Big blind amount |
| `buy_in_min` | number | 50×BB | Minimum buy-in |
| `buy_in_max` | number | 500×BB | Maximum buy-in |
| `auto_start` | bool | true | Start game with 2+ players |



## Client Configuration (`holdem-client.hcl`)

### Basic Structure

```hcl
server {
  url               = "http://localhost:8080"
  connect_timeout   = 10
  request_timeout   = 30
  reconnect_attempts = 3
  reconnect_delay   = 5
}

player {
  name             = "PlayerName"
  default_buy_in   = 200
  auto_rebuy       = false
  rebuy_threshold  = 50
}

ui {
  log_level         = "info"
  log_file          = "holdem-client.log"
  show_hole_cards   = true
  show_bot_thinking = false
  auto_scroll_log   = true
  confirm_actions   = false
  theme             = "default"
}
```

### Server Block

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `url` | string | **required** | Server WebSocket URL |
| `connect_timeout` | number | 10 | Connection timeout (seconds) |
| `request_timeout` | number | 30 | Request timeout (seconds) |
| `reconnect_attempts` | number | 3 | Max reconnection attempts |
| `reconnect_delay` | number | 5 | Delay between attempts (seconds) |

### Player Block

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `name` | string | **required** | Player display name |
| `default_buy_in` | number | 200 | Default table buy-in |
| `auto_rebuy` | bool | false | Auto-rebuy when low |
| `rebuy_threshold` | number | 50 | Chips level for auto-rebuy |

### UI Block

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `log_level` | string | "info" | Log level: debug, info, warn, error |
| `log_file` | string | "holdem-client.log" | Log file path |
| `show_hole_cards` | bool | true | Show your cards in UI |
| `show_bot_thinking` | bool | false | Show AI reasoning |
| `auto_scroll_log` | bool | true | Auto-scroll game log |
| `confirm_actions` | bool | false | Confirm large bets |
| `theme` | string | "default" | UI theme: default, dark, light |

## Usage

### Server

```bash
# Use default config file
./bin/holdem-server

# Specify config file
./bin/holdem-server --config=configs/server-tournament.hcl

# Override config with flags
./bin/holdem-server --config=my-server.hcl --addr=0.0.0.0:9000 --log-level=debug

# Legacy mode (ignore config file)
./bin/holdem-server --tables=3
```

### Client

```bash
# Use default config file
./bin/holdem-client

# Specify config file
./bin/holdem-client --config=configs/client-pro.hcl

# Override config with flags
./bin/holdem-client --config=my-client.hcl --server=http://remote:8080 --player=Alice

# Legacy mode (no config file)
./bin/holdem-client --server=http://localhost:8080 --player=Bob
```

## Example Configurations

### Multi-Stakes Server

```hcl
server {
  address = "0.0.0.0"
  port    = 8080
}

table "microstakes" {
  small_blind = 1
  big_blind   = 2
  buy_in_min  = 40
  buy_in_max  = 200
}

table "midstakes" {
  small_blind = 5
  big_blind   = 10
  buy_in_min  = 200
  buy_in_max  = 1000
}

table "highstakes" {
  small_blind = 25
  big_blind   = 50
  buy_in_min  = 1000
  buy_in_max  = 10000
}
```

### Tournament Player Client

```hcl
server {
  url = "ws://tournament.poker.com:8080"
}

player {
  name           = "TournamentPlayer"
  default_buy_in = 1500
  auto_rebuy     = true
}

ui {
  log_level       = "debug"
  show_bot_thinking = true
  confirm_actions = true
  theme          = "dark"
}
```

## Configuration Validation

Both server and client validate configuration on startup:

- **Server**: Validates port ranges, table limits
- **Client**: Validates URLs, timeouts, log levels, themes

Invalid configurations will show helpful error messages and exit.

## Environment Variables

You can use environment variables in HCL files:

```hcl
server {
  address = "${SERVER_HOST}"
  port    = "${SERVER_PORT}"
}

player {
  name = "${PLAYER_NAME}"
}
```

Set them before running:

```bash
export SERVER_HOST=0.0.0.0
export SERVER_PORT=9000
export PLAYER_NAME=Alice
./bin/holdem-server
```

## Migration from Command Line

The new HCL configuration is fully backward compatible. Existing command-line usage continues to work:

```bash
# This still works (legacy mode)
./bin/holdem-server --tables=2 --addr=localhost:9000

# But this is more flexible (HCL mode)
./bin/holdem-server --config=my-server.hcl
```

When using legacy command-line flags, the configuration file is ignored for those specific settings.

package client

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// ClientConfig represents the complete client configuration
type ClientConfig struct {
	Server ServerConnection `hcl:"server,block"`
	Player PlayerSettings   `hcl:"player,block"`
	UI     UISettings       `hcl:"ui,block"`
}

// ServerConnection contains server connection settings
type ServerConnection struct {
	URL               string `hcl:"url"`
	ConnectTimeout    int    `hcl:"connect_timeout,optional"`
	RequestTimeout    int    `hcl:"request_timeout,optional"`
	ReconnectAttempts int    `hcl:"reconnect_attempts,optional"`
	ReconnectDelay    int    `hcl:"reconnect_delay,optional"`
}

// PlayerSettings contains player-specific settings
type PlayerSettings struct {
	Name           string `hcl:"name"`
	DefaultBuyIn   int    `hcl:"default_buy_in,optional"`
	AutoRebuy      bool   `hcl:"auto_rebuy,optional"`
	RebuyThreshold int    `hcl:"rebuy_threshold,optional"`
}

// UISettings contains user interface settings
type UISettings struct {
	LogLevel        string `hcl:"log_level,optional"`
	LogFile         string `hcl:"log_file,optional"`
	ShowHoleCards   bool   `hcl:"show_hole_cards,optional"`
	ShowBotThinking bool   `hcl:"show_bot_thinking,optional"`
	AutoScrollLog   bool   `hcl:"auto_scroll_log,optional"`
	ConfirmActions  bool   `hcl:"confirm_actions,optional"`
	Theme           string `hcl:"theme,optional"`
}

// DefaultClientConfig returns default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Server: ServerConnection{
			URL:               "http://localhost:8080",
			ConnectTimeout:    10,
			RequestTimeout:    30,
			ReconnectAttempts: 3,
			ReconnectDelay:    5,
		},
		Player: PlayerSettings{
			Name:           "",
			DefaultBuyIn:   200,
			AutoRebuy:      false,
			RebuyThreshold: 50,
		},
		UI: UISettings{
			LogLevel:        "warn",
			LogFile:         "holdem-client.log",
			ShowHoleCards:   true,
			ShowBotThinking: false,
			AutoScrollLog:   true,
			ConfirmActions:  false,
			Theme:           "default",
		},
	}
}

// LoadClientConfig loads client configuration from HCL file
func LoadClientConfig(filename string) (*ClientConfig, error) {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return DefaultClientConfig(), nil
	}

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file: %s", diags.Error())
	}

	var config ClientConfig
	diags = gohcl.DecodeBody(file.Body, nil, &config)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL: %s", diags.Error())
	}

	// Apply defaults for missing values
	defaults := DefaultClientConfig()

	if config.Server.URL == "" {
		config.Server.URL = defaults.Server.URL
	}
	if config.Server.ConnectTimeout == 0 {
		config.Server.ConnectTimeout = defaults.Server.ConnectTimeout
	}
	if config.Server.RequestTimeout == 0 {
		config.Server.RequestTimeout = defaults.Server.RequestTimeout
	}
	if config.Server.ReconnectAttempts == 0 {
		config.Server.ReconnectAttempts = defaults.Server.ReconnectAttempts
	}
	if config.Server.ReconnectDelay == 0 {
		config.Server.ReconnectDelay = defaults.Server.ReconnectDelay
	}

	if config.Player.DefaultBuyIn == 0 {
		config.Player.DefaultBuyIn = defaults.Player.DefaultBuyIn
	}
	if config.Player.RebuyThreshold == 0 {
		config.Player.RebuyThreshold = defaults.Player.RebuyThreshold
	}

	if config.UI.LogLevel == "" {
		config.UI.LogLevel = defaults.UI.LogLevel
	}
	if config.UI.LogFile == "" {
		config.UI.LogFile = defaults.UI.LogFile
	}
	if config.UI.Theme == "" {
		config.UI.Theme = defaults.UI.Theme
	}

	return &config, nil
}

// Validate validates the client configuration
func (c *ClientConfig) Validate() error {
	if c.Server.URL == "" {
		return fmt.Errorf("server URL is required")
	}

	if c.Player.Name == "" {
		return fmt.Errorf("player name is required")
	}

	if c.Player.DefaultBuyIn <= 0 {
		return fmt.Errorf("default buy-in must be positive")
	}

	if c.Server.ConnectTimeout <= 0 {
		return fmt.Errorf("connect timeout must be positive")
	}

	if c.Server.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive")
	}

	if c.Server.ReconnectAttempts < 0 {
		return fmt.Errorf("reconnect attempts cannot be negative")
	}

	if c.Server.ReconnectDelay <= 0 {
		return fmt.Errorf("reconnect delay must be positive")
	}

	if c.Player.RebuyThreshold < 0 {
		return fmt.Errorf("rebuy threshold cannot be negative")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.UI.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.UI.LogLevel)
	}

	// Validate theme
	validThemes := map[string]bool{
		"default": true,
		"dark":    true,
		"light":   true,
	}
	if !validThemes[c.UI.Theme] {
		return fmt.Errorf("invalid theme: %s", c.UI.Theme)
	}

	return nil
}

// GetLogLevel returns the log level
func (c *ClientConfig) GetLogLevel() string {
	return c.UI.LogLevel
}

// GetServerURL returns the server URL
func (c *ClientConfig) GetServerURL() string {
	return c.Server.URL
}

// GetPlayerName returns the player name
func (c *ClientConfig) GetPlayerName() string {
	return c.Player.Name
}

// ShouldAutoRebuy returns whether auto-rebuy is enabled
func (c *ClientConfig) ShouldAutoRebuy(currentChips int) bool {
	return c.Player.AutoRebuy && currentChips <= c.Player.RebuyThreshold
}

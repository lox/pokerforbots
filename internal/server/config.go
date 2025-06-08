package server

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// ServerConfig represents the complete server configuration
type ServerConfig struct {
	Server ServerSettings `hcl:"server,block"`
	Tables []TableConfig  `hcl:"table,block"`
}

// ServerSettings contains server-level configuration
type ServerSettings struct {
	Address  string `hcl:"address,optional"`
	Port     int    `hcl:"port,optional"`
	LogLevel string `hcl:"log_level,optional"`
	LogFile  string `hcl:"log_file,optional"`
}

// TableConfig defines a poker table configuration
type TableConfig struct {
	Name           string `hcl:"name,label"`
	MaxPlayers     int    `hcl:"max_players,optional"`
	SmallBlind     int    `hcl:"small_blind"`
	BigBlind       int    `hcl:"big_blind"`
	BuyInMin       int    `hcl:"buy_in_min,optional"`
	BuyInMax       int    `hcl:"buy_in_max,optional"`
	AutoStart      bool   `hcl:"auto_start,optional"`
	TimeoutSeconds int    `hcl:"timeout_seconds,optional"`
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Server: ServerSettings{
			Address:  "localhost",
			Port:     8080,
			LogLevel: "info",
			LogFile:  "holdem-server.log",
		},
		Tables: []TableConfig{
			{
				Name:           "main",
				MaxPlayers:     6,
				SmallBlind:     1,
				BigBlind:       2,
				BuyInMin:       100,
				BuyInMax:       1000,
				AutoStart:      true,
				TimeoutSeconds: 60,
			},
		},
	}
}

// LoadServerConfig loads server configuration from HCL file
func LoadServerConfig(filename string) (*ServerConfig, error) {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return DefaultServerConfig(), nil
	}

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file: %s", diags.Error())
	}

	var config ServerConfig
	diags = gohcl.DecodeBody(file.Body, nil, &config)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL: %s", diags.Error())
	}

	// Apply defaults for missing values
	if config.Server.Address == "" {
		config.Server.Address = "localhost"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.LogLevel == "" {
		config.Server.LogLevel = "info"
	}
	if config.Server.LogFile == "" {
		config.Server.LogFile = "holdem-server.log"
	}

	// Apply defaults to tables
	for i := range config.Tables {
		if config.Tables[i].MaxPlayers == 0 {
			config.Tables[i].MaxPlayers = 6
		}
		if config.Tables[i].BuyInMin == 0 {
			config.Tables[i].BuyInMin = config.Tables[i].BigBlind * 50 // 50 big blinds minimum
		}
		if config.Tables[i].BuyInMax == 0 {
			config.Tables[i].BuyInMax = config.Tables[i].BigBlind * 500 // 500 big blinds maximum
		}
		if config.Tables[i].TimeoutSeconds == 0 {
			config.Tables[i].TimeoutSeconds = 60 // 60 seconds default
		}
	}

	return &config, nil
}

// Validate validates the server configuration
func (c *ServerConfig) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}

	if len(c.Tables) == 0 {
		return fmt.Errorf("at least one table must be configured")
	}

	for _, table := range c.Tables {
		if table.SmallBlind <= 0 {
			return fmt.Errorf("table %s: small blind must be positive", table.Name)
		}
		if table.BigBlind <= table.SmallBlind {
			return fmt.Errorf("table %s: big blind must be greater than small blind", table.Name)
		}
		if table.MaxPlayers < 2 || table.MaxPlayers > 10 {
			return fmt.Errorf("table %s: max players must be between 2 and 10", table.Name)
		}
		if table.BuyInMin >= table.BuyInMax {
			return fmt.Errorf("table %s: buy-in minimum must be less than maximum", table.Name)
		}
		if table.TimeoutSeconds < 10 || table.TimeoutSeconds > 300 {
			return fmt.Errorf("table %s: timeout must be between 10 and 300 seconds", table.Name)
		}
	}

	return nil
}

// GetServerAddress returns the full server address
func (c *ServerConfig) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Address, c.Server.Port)
}

// GetTableByName returns a table configuration by name
func (c *ServerConfig) GetTableByName(name string) *TableConfig {
	for _, table := range c.Tables {
		if table.Name == name {
			return &table
		}
	}
	return nil
}

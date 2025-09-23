package regression

import (
	"fmt"
	"strconv"
	"strings"
)

// ServerConfig holds configuration for starting a test server
type ServerConfig struct {
	// Core settings
	Seed      int64
	Hands     int
	StatsFile string

	// Bot configuration
	BotCommands []string
	NPCCommands []string
	NPCConfig   string // Built-in NPC configuration string

	// Optional settings (will use defaults from Config if not set)
	Addr             string
	StartingChips    int
	TimeoutMs        int
	InfiniteBankroll bool
}

// BuildServerArgs constructs command-line arguments from the configuration
func (sc *ServerConfig) BuildServerArgs(defaults *Config) []string {
	args := []string{
		"--addr", sc.getAddr(defaults),
		"--start-chips", fmt.Sprintf("%d", sc.getStartingChips(defaults)),
		"--timeout-ms", fmt.Sprintf("%d", sc.getTimeoutMs(defaults)),
		"--seed", fmt.Sprintf("%d", sc.Seed),
		"--hands", fmt.Sprintf("%d", sc.Hands),
		"--collect-detailed-stats",
	}

	// Add stats output file if specified
	if sc.StatsFile != "" {
		args = append(args, "--write-stats-on-exit", sc.StatsFile)
	}

	// Add infinite bankroll flag if set
	if sc.InfiniteBankroll || defaults.InfiniteBankroll {
		args = append(args, "--infinite-bankroll")
	}

	// Add stop on insufficient bots flag if set
	if defaults.StopOnInsufficientBots {
		args = append(args, "--stop-on-insufficient-bots")
	}

	// Add bot commands
	for _, cmd := range sc.BotCommands {
		args = append(args, "--bot-cmd", cmd)
	}

	// Add NPC commands
	for _, cmd := range sc.NPCCommands {
		args = append(args, "--npc-bot-cmd", cmd)
	}

	// Add built-in NPC configuration
	if sc.NPCConfig != "" {
		args = append(args, "--npcs", sc.NPCConfig)
	}

	return args
}

// Helper methods to get values with defaults
func (sc *ServerConfig) getAddr(defaults *Config) string {
	if sc.Addr != "" {
		return sc.Addr
	}
	if defaults.ServerAddr != "" && defaults.ServerAddr != "embedded" {
		return defaults.ServerAddr
	}
	return "localhost:8080"
}

func (sc *ServerConfig) getStartingChips(defaults *Config) int {
	if sc.StartingChips > 0 {
		return sc.StartingChips
	}
	if defaults.StartingChips > 0 {
		return defaults.StartingChips
	}
	return 1000 // Default starting chips
}

func (sc *ServerConfig) getTimeoutMs(defaults *Config) int {
	if sc.TimeoutMs > 0 {
		return sc.TimeoutMs
	}
	if defaults.TimeoutMs > 0 {
		return defaults.TimeoutMs
	}
	return 100 // Default timeout
}

// CountNPCs counts the number of NPCs from the configuration
func (sc *ServerConfig) CountNPCs() int {
	count := len(sc.NPCCommands)

	// Count NPCs from built-in config string
	if sc.NPCConfig != "" {
		for part := range strings.SplitSeq(sc.NPCConfig, ",") {
			if colonPos := strings.Index(part, ":"); colonPos > 0 {
				if countStr := strings.TrimSpace(part[colonPos+1:]); countStr != "" {
					if n, err := strconv.Atoi(countStr); err == nil {
						count += n
					}
				}
			}
		}
	}

	return count
}

// BuildReproCommand generates the exact `task server` command to reproduce this batch
func (sc *ServerConfig) BuildReproCommand(defaults *Config) string {
	args := sc.BuildServerArgs(defaults)

	// Quote arguments that contain spaces
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		if strings.Contains(arg, " ") {
			quotedArgs[i] = fmt.Sprintf("'%s'", arg)
		} else {
			quotedArgs[i] = arg
		}
	}

	return "task server -- " + strings.Join(quotedArgs, " ")
}

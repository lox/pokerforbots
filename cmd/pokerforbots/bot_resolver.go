package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ExecutionMode int

const (
	ModeBinary ExecutionMode = iota
	ModeGoRun
)

func (m ExecutionMode) String() string {
	switch m {
	case ModeBinary:
		return "binary"
	case ModeGoRun:
		return "go run"
	default:
		return "unknown"
	}
}

type BotResolver struct {
	mode            ExecutionMode
	pokerforbotsBin string
	projectRoot     string
}

func NewBotResolver() *BotResolver {
	r := &BotResolver{}
	r.detectMode()
	return r
}

func (r *BotResolver) detectMode() {
	// Check if we're running via 'go run'
	executable, _ := os.Executable()

	// go run creates binaries in temp directories like:
	// - /tmp/go-build1234/b001/exe/main
	// - /var/folders/.../T/go-build.../exe/main
	// - On Windows: C:\Users\...\AppData\Local\Temp\go-build...\exe\main.exe
	if strings.Contains(executable, "/go-build") ||
		strings.Contains(executable, "\\go-build") ||
		strings.Contains(executable, "/T/go_build") ||
		strings.Contains(executable, "\\Temp\\go-build") {
		r.mode = ModeGoRun

		// Find project root by looking for go.mod
		r.projectRoot = r.findProjectRoot()
	} else {
		r.mode = ModeBinary
		r.pokerforbotsBin = executable
	}
}

func (r *BotResolver) findProjectRoot() string {
	// Start from current directory and walk up
	dir, _ := os.Getwd()
	for dir != "/" && dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return ""
}

func (r *BotResolver) Mode() ExecutionMode {
	return r.mode
}

func (r *BotResolver) ProjectRoot() string {
	return r.projectRoot
}

func (r *BotResolver) Binary() string {
	return r.pokerforbotsBin
}

// Resolve returns the command and arguments to run a bot
func (r *BotResolver) Resolve(botName string) (command string, args []string, err error) {
	// If it's already a path/command, use as-is
	if strings.Contains(botName, "/") || strings.Contains(botName, ".") {
		return r.parseDirectCommand(botName)
	}

	switch r.mode {
	case ModeGoRun:
		// Development mode - use go run
		return r.resolveGoRun(botName)
	case ModeBinary:
		// Production mode - use embedded sub-commands
		return r.resolveBinary(botName)
	}

	return "", nil, fmt.Errorf("unknown bot: %s", botName)
}

func (r *BotResolver) parseDirectCommand(cmd string) (string, []string, error) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("empty command")
	}
	return fields[0], fields[1:], nil
}

func (r *BotResolver) resolveGoRun(botName string) (string, []string, error) {
	// Map bot names to their source paths
	botPaths := map[string]string{
		"calling-station": "./sdk/examples/calling-station",
		"calling":         "./sdk/examples/calling-station",
		"cs":              "./sdk/examples/calling-station",
		"random":          "./sdk/examples/random",
		"rnd":             "./sdk/examples/random",
		"aggressive":      "./sdk/examples/aggressive",
		"aggro":           "./sdk/examples/aggressive",
		"complex":         "./sdk/examples/complex",
	}

	path, ok := botPaths[strings.ToLower(botName)]
	if !ok {
		return "", nil, fmt.Errorf("unknown bot: %s", botName)
	}

	// Build the full path if we found project root
	if r.projectRoot != "" {
		path = filepath.Join(r.projectRoot, path)
	}
	return "go", []string{"run", path}, nil
}

func (r *BotResolver) resolveBinary(botName string) (string, []string, error) {
	// Map bot names to embedded sub-commands
	botCommands := map[string]string{
		"calling-station": "calling-station",
		"calling":         "calling-station",
		"cs":              "calling-station",
		"random":          "random",
		"rnd":             "random",
		"aggressive":      "aggressive",
		"aggro":           "aggressive",
		"complex":         "complex",
	}

	subCmd, ok := botCommands[strings.ToLower(botName)]
	if !ok {
		return "", nil, fmt.Errorf("unknown bot: %s", botName)
	}

	return r.pokerforbotsBin, []string{"bots", subCmd}, nil
}

// ResolveWithServer provides context-aware bot command with WebSocket URL
func (r *BotResolver) ResolveWithServer(botName, wsURL string) (string, []string, error) {
	cmd, args, err := r.Resolve(botName)
	if err != nil {
		return "", nil, err
	}

	// Append the WebSocket URL
	args = append(args, wsURL)
	return cmd, args, nil
}

// ListAvailableBots returns the list of known bot names
func (r *BotResolver) ListAvailableBots() []string {
	return []string{
		"calling-station (aliases: calling, cs)",
		"random (aliases: rnd)",
		"aggressive (aliases: aggro)",
		"complex",
	}
}

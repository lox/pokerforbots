package complex

import (
	"fmt"
	"os"
	"os/exec"
)

// Run starts the complex bot by executing the external binary
// The complex bot is more sophisticated and is typically compiled separately
func Run(serverURL, name, game string) error {
	// For now, complex bot runs the actual example if it exists
	// This is a placeholder that attempts to run the complex bot example

	// Try to find and run the complex bot
	paths := []string{
		"./sdk/examples/complex",
		"./dist/complex-bot",
		"complex-bot",
	}

	for _, path := range paths {
		// Check if path exists and is executable
		if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue // Skip if not found, is a directory, or not executable
		}

		cmd := exec.Command(path, "--server", serverURL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Set environment variables
		env := os.Environ()
		if name != "" && name != "ComplexBot" {
			env = append(env, fmt.Sprintf("BOT_NAME=%s", name))
		}
		if game != "" && game != "default" {
			env = append(env, fmt.Sprintf("POKERFORBOTS_GAME=%s", game))
		}
		cmd.Env = env

		err := cmd.Run()
		if err == nil {
			return nil
		}

		return fmt.Errorf("complex bot failed: %w", err)
	}

	// If we can't find the complex bot, try go run as fallback
	cmd := exec.Command("go", "run", "./sdk/examples/complex", "--server", serverURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	if name != "" && name != "ComplexBot" {
		env = append(env, fmt.Sprintf("BOT_NAME=%s", name))
	}
	if game != "" && game != "default" {
		env = append(env, fmt.Sprintf("POKERFORBOTS_GAME=%s", game))
	}
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("complex bot not found or failed to run: %w", err)
	}

	return nil
}

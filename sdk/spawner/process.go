package spawner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Process represents a managed bot process.
type Process struct {
	ID      string
	Command string
	Args    []string
	Env     map[string]string

	cmd       *exec.Cmd
	ctx       context.Context
	cancel    context.CancelFunc
	logger    zerolog.Logger
	startTime time.Time
	endTime   time.Time
	mu        sync.RWMutex
	done      chan struct{}
	exitErr   error
}

// NewProcess creates a new process manager.
func NewProcess(ctx context.Context, command string, args []string, env map[string]string, logger zerolog.Logger) *Process {
	procCtx, cancel := context.WithCancel(ctx)
	id := uuid.NewString()[:8]

	return &Process{
		ID:      id,
		Command: command,
		Args:    args,
		Env:     env,
		ctx:     procCtx,
		cancel:  cancel,
		logger:  logger.With().Str("process_id", id).Logger(),
		done:    make(chan struct{}),
	}
}

// Start starts the process.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil {
		return fmt.Errorf("process already started")
	}

	// Create command with context
	p.cmd = exec.CommandContext(p.ctx, p.Command, p.Args...)

	// Set environment
	p.cmd.Env = os.Environ() // Inherit parent environment
	for k, v := range p.Env {
		p.cmd.Env = append(p.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create pipes for stdout/stderr
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	p.startTime = time.Now()
	p.logger.Info().
		Str("command", p.Command).
		Strs("args", p.Args).
		Msg("Process started")

	// Start output readers
	go p.readOutput("stdout", stdout)
	go p.readOutput("stderr", stderr)

	// Start process monitor
	go p.monitor()

	return nil
}

// Stop stops the process gracefully, then forcefully if needed.
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return nil // Not started
	}

	// Check if already done
	select {
	case <-p.done:
		return nil // Already stopped
	default:
	}

	// Try graceful interrupt first
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		// Process might already be gone, which is fine
		select {
		case <-p.done:
			return nil
		default:
			// If not done and signal failed, try kill
			if err := p.cmd.Process.Kill(); err != nil {
				// Check again if process finished
				select {
				case <-p.done:
					return nil
				default:
					return fmt.Errorf("failed to stop process: %w", err)
				}
			}
		}
	}

	// Wait briefly for process to exit after signal
	select {
	case <-p.done:
		return nil
	case <-time.After(1 * time.Second):
		// Force kill if not stopped
		p.logger.Debug().Msg("Force killing process")
		if err := p.cmd.Process.Kill(); err != nil {
			// Check if process already exited
			select {
			case <-p.done:
				return nil
			default:
				// Check if error is "process already finished"
				if strings.Contains(err.Error(), "process already finished") {
					// Process already dead, that's fine
					return nil
				}
				return fmt.Errorf("failed to kill process: %w", err)
			}
		}
		<-p.done
	}

	return nil
}

// Wait waits for the process to exit.
func (p *Process) Wait() error {
	<-p.done
	return p.exitErr
}

// IsAlive returns true if the process is still running.
func (p *Process) IsAlive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// monitor monitors the process and waits for it to exit.
func (p *Process) monitor() {
	defer close(p.done)

	err := p.cmd.Wait()

	p.mu.Lock()
	p.endTime = time.Now()
	p.exitErr = err
	p.mu.Unlock()

	if err != nil {
		// Check if this was a signal termination (expected during shutdown)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.String() == "signal: killed" || exitErr.String() == "signal: terminated" || exitErr.String() == "signal: interrupt" {
				// Expected during shutdown, log as info not error
				p.logger.Info().
					Dur("duration", time.Since(p.startTime)).
					Msg("Process terminated by signal")
			} else {
				p.logger.Error().
					Err(err).
					Dur("duration", time.Since(p.startTime)).
					Msg("Process exited with error")
			}
		} else {
			p.logger.Error().
				Err(err).
				Dur("duration", time.Since(p.startTime)).
				Msg("Process exited with error")
		}
	} else {
		p.logger.Info().
			Dur("duration", time.Since(p.startTime)).
			Msg("Process exited successfully")
	}
}

// readOutput reads and logs output from a pipe.
func (p *Process) readOutput(name string, pipe io.Reader) {
	scanner := bufio.NewScanner(pipe)
	prefix := fmt.Sprintf("[%s:%s] ", p.ID[:4], name[:3])

	for scanner.Scan() {
		line := scanner.Text()
		// Log bot output at Info level for stderr (where most output goes), Debug for stdout
		if name == "stderr" && len(line) > 0 {
			// Try to extract message from JSON logs for cleaner output
			message := line
			if strings.Contains(line, `"message":"`) {
				// Simple extraction of message field from JSON
				start := strings.Index(line, `"message":"`) + 11
				if end := strings.Index(line[start:], `"`); end > 0 {
					message = fmt.Sprintf("[Bot %s] %s", p.ID[:8], line[start:start+end])
				}
			}
			p.logger.Info().Msg(message)
		} else if len(line) > 0 {
			p.logger.Debug().
				Str("stream", name).
				Str("output", line).
				Msg(prefix + line)
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		// Check if this is an expected error due to process termination
		errStr := err.Error()
		if strings.Contains(errStr, "file already closed") || strings.Contains(errStr, "broken pipe") {
			// Expected when process is terminated, don't log as error
			return
		}

		// Only log actual errors, not expected pipe closures
		select {
		case <-p.done:
			// Process is done, pipe closure is expected
		default:
			p.logger.Error().
				Err(err).
				Str("stream", name).
				Msg("Error reading output")
		}
	}
}

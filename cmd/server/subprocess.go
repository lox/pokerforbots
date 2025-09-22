package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/rs/zerolog"
)

// spawnBotProcess starts a bot subprocess with the given command and environment.
// It returns a channel that will receive the process exit error (or nil) when complete.
func spawnBotProcess(ctx context.Context, logger zerolog.Logger, cmdStr string, env []string, prefix string) <-chan error {
	done := make(chan error, 1)

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	cmd.Env = append(os.Environ(), env...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error().Err(err).Str("cmd", cmdStr).Msg("Failed to create stdout pipe")
		done <- err
		close(done)
		return done
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Error().Err(err).Str("cmd", cmdStr).Msg("Failed to create stderr pipe")
		done <- err
		close(done)
		return done
	}

	logger.Info().Str("cmd", cmdStr).Str("prefix", prefix).Msg("Starting bot process")
	if err := cmd.Start(); err != nil {
		logger.Error().Err(err).Str("cmd", cmdStr).Msg("Failed to start bot process")
		done <- err
		close(done)
		return done
	}

	// Use sync.WaitGroup to ensure pipes are fully drained
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		copyWithPrefix(os.Stdout, stdout, prefix)
		stdout.Close() // Important: close the pipe when done reading
	}()

	go func() {
		defer wg.Done()
		copyWithPrefix(os.Stderr, stderr, prefix)
		stderr.Close() // Important: close the pipe when done reading
	}()

	go func() {
		// Wait for the process to exit
		cmdErr := cmd.Wait()

		// Wait for all output to be consumed
		wg.Wait()

		if cmdErr != nil {
			logger.Error().Err(cmdErr).Str("cmd", cmdStr).Msg("Bot process exited with error")
			done <- cmdErr
		} else {
			logger.Info().Str("cmd", cmdStr).Msg("Bot process exited")
			done <- nil
		}
		close(done)
	}()
	return done
}

// copyWithPrefix reads from src and writes to dst with a prefix on each line
func copyWithPrefix(dst *os.File, src io.ReadCloser, prefix string) {
	s := bufio.NewScanner(src)
	for s.Scan() {
		fmt.Fprintln(dst, prefix+s.Text())
	}
}

// firstDone returns a channel that receives from the first channel in chans to send a value
func firstDone(chans []<-chan error) <-chan error {
	if len(chans) == 0 {
		return nil
	}
	out := make(chan error, 1)
	for _, ch := range chans {
		c := ch
		go func() {
			if err := <-c; err != nil {
				out <- err
			} else {
				out <- nil
			}
		}()
	}
	return out
}

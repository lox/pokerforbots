// Package fileutil provides file system utilities.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to a file atomically by writing to a temporary file
// and then renaming it to the final path. This ensures readers never see partial
// writes - they see either no file or the complete file.
//
// The atomic rename is guaranteed by POSIX. Readers will observe:
// - No file (not ready)
// - Complete file (fully written and renamed)
// - Never a partial file
func WriteFileAtomic(filename string, data []byte, perm os.FileMode) error {
	// Create temp file in same directory to ensure it's on same filesystem
	// (cross-filesystem renames are not atomic)
	dir := filepath.Dir(filename)
	base := filepath.Base(filename)

	tmpFile, err := os.CreateTemp(dir, base+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure temp file is cleaned up on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync to ensure data is on disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tmpFile = nil // Prevent defer cleanup

	// Set correct permissions
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename (POSIX guarantees atomicity)
	if err := os.Rename(tmpPath, filename); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

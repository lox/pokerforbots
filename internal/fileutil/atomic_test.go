package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("hello world")

	// Write atomically
	err := WriteFileAtomic(testFile, testData, 0644)
	if err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("File content mismatch: got %q, want %q", string(data), string(testData))
	}

	// Verify permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0644 {
		t.Errorf("File permissions mismatch: got %o, want %o", info.Mode().Perm(), 0644)
	}

	// Verify no temp files remain
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	for _, entry := range entries {
		if entry.Name() != "test.txt" {
			t.Errorf("Unexpected file in directory: %s", entry.Name())
		}
	}
}

func TestWriteFileAtomicOverwrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Write initial content
	err := WriteFileAtomic(testFile, []byte("initial"), 0644)
	if err != nil {
		t.Fatalf("Initial write failed: %v", err)
	}

	// Overwrite with new content
	newData := []byte("updated content")
	err = WriteFileAtomic(testFile, newData, 0644)
	if err != nil {
		t.Fatalf("Overwrite failed: %v", err)
	}

	// Verify new content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != string(newData) {
		t.Errorf("File content mismatch: got %q, want %q", string(data), string(newData))
	}
}

func TestWriteFileAtomicInvalidDir(t *testing.T) {
	t.Parallel()

	// Try to write to non-existent directory
	err := WriteFileAtomic("/nonexistent/dir/test.txt", []byte("data"), 0644)
	if err == nil {
		t.Error("Expected error when writing to non-existent directory")
	}
}

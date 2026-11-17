package filelock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireCreatesLockFileAndCanBeReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.lock")
	lock, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if lock == nil {
		t.Fatal("Acquire() returned a nil lock")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file was not created at requested path: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestAcquireRejectsBlankPath(t *testing.T) {
	if _, err := Acquire(" \t "); !errors.Is(err, ErrPathRequired) {
		t.Fatalf("Acquire(blank) error = %v, want ErrPathRequired", err)
	}
}

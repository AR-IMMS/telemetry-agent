package lifecyclelock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireTrimsAndCleansConfigPathBeforeAppendingLockSuffix(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "nested", "..", "collector.yaml")
	lock, err := Acquire("  " + configPath + "  ")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer lock.Close()

	wantPath := filepath.Clean(configPath) + ".lock"
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("lifecycle lock was not created at %q: %v", wantPath, err)
	}
}

//go:build !windows

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestActivateRenderedConfigPreservesExistingFileMode(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "collector.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ActivateRenderedConfig(context.Background(), &activationRunner{result: CommandResult{}}, "/fake/collector", targetPath, []byte("new"), nil); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("replacement mode = %#o, want prior restrictive mode %#o", got, 0o600)
	}
}

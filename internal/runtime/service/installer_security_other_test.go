//go:build !windows

package agentinstallation

import (
	"fmt"
	"os"
	"testing"
)

func setConfigSecurityForTest(t *testing.T, path string) string {
	t.Helper()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return captureConfigSecurityForTest(t, path)
}

func captureConfigSecurityForTest(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%#o", info.Mode().Perm())
}

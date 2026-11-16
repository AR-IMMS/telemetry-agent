//go:build !windows

package agentlifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func trustManagedRuntimeCertificate(t *testing.T, certificate []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pr07c-test-root.pem")
	if err := os.WriteFile(path, certificate, 0600); err != nil {
		t.Fatal(err)
	}
	previous, existed := os.LookupEnv("SSL_CERT_FILE")
	if err := os.Setenv("SSL_CERT_FILE", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("SSL_CERT_FILE", previous)
		} else {
			_ = os.Unsetenv("SSL_CERT_FILE")
		}
	})
}

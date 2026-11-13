//go:build windows

package agentlifecycle

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func trustManagedRuntimeCertificate(t *testing.T, certificate []byte) {
	t.Helper()
	block, _ := pem.Decode(certificate)
	if block == nil {
		t.Fatal("decode temporary Supervisor trust certificate")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pr07c-test-root.cer")
	if err := os.WriteFile(path, certificate, 0600); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("certutil.exe", "-user", "-f", "-addstore", "Root", path)
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("add temporary test certificate to current-user trust store: %v\n%s", err, output)
	}
	fingerprint := sha1.Sum(parsed.Raw)
	thumbprint := strings.ToUpper(hex.EncodeToString(fingerprint[:]))
	t.Cleanup(func() {
		remove := exec.Command("certutil.exe", "-user", "-delstore", "Root", thumbprint)
		if output, err := remove.CombinedOutput(); err != nil {
			t.Errorf("remove temporary test certificate from current-user trust store: %v\n%s", err, output)
		}
	})
}

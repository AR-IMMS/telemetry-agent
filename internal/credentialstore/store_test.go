//go:build linux

package credentialstore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

func TestStorePersistsAcrossRestartAndReplacesAtomically(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	store := New(statePath)
	if err := store.Put("bootstrap/attempt-1", "first-secret"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Put("bootstrap/attempt-1", "second-secret"); err != nil {
		t.Fatalf("replacement Put() error = %v", err)
	}
	restarted := New(statePath)
	got, err := restarted.Get("bootstrap/attempt-1")
	if err != nil || got != "second-secret" {
		t.Fatalf("Get() = %q, %v", got, err)
	}
	if err := restarted.Delete("bootstrap/attempt-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := restarted.Delete("bootstrap/attempt-1"); err != nil {
		t.Fatalf("repeated Delete() error = %v", err)
	}
	if _, err := restarted.Get("bootstrap/attempt-1"); err != enrollment.ErrCredentialNotFound {
		t.Fatalf("Get() error = %v, want ErrCredentialNotFound", err)
	}
}

func TestStoreProtectsSecretFileForTargetOS(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	store := New(statePath)
	secret := "do-not-store-in-sqlite"
	if err := store.Put("access/installation-1", secret); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	content, err := os.ReadFile(store.path("access/installation-1"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && strings.Contains(string(content), secret) {
		t.Fatal("Windows credential file contains plaintext secret")
	}
	if runtime.GOOS == "linux" {
		if string(content) != secret {
			t.Fatal("Linux credential file does not contain the root/service-owned value")
		}
		directory, err := os.Stat(store.directory)
		if err != nil || directory.Mode().Perm() != 0o700 {
			t.Fatalf("credential directory mode=%v err=%v, want 0700", directory.Mode().Perm(), err)
		}
		file, err := os.Stat(store.path("access/installation-1"))
		if err != nil || file.Mode().Perm() != 0o600 {
			t.Fatalf("credential file mode=%v err=%v, want 0600", file.Mode().Perm(), err)
		}
	}
}

func TestStoreRejectsInvalidPathAndSecret(t *testing.T) {
	if err := (Store{}).Put("reference", "secret"); err == nil {
		t.Fatal("Put() error = nil for missing store path")
	}
	store := New(filepath.Join(t.TempDir(), "state.db"))
	if err := store.Put(" ", "secret"); err == nil {
		t.Fatal("Put() error = nil for empty reference")
	}
	if err := store.Put("reference", " "); err == nil {
		t.Fatal("Put() error = nil for empty secret")
	}
}

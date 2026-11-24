package credentials

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProvisionTokenFileWritesAndReplacesProtectedFile(t *testing.T) {
	if runtime.GOOS == "windows" && !canPersistPlatformCredentialForTest() {
		t.Skip("Windows Supervisor token-file integration requires an elevated process")
	}
	statePath := filepath.Join(t.TempDir(), "state.db")
	store := New(statePath)
	if err := store.Put("agent-access", "first-secret-token"); err != nil {
		t.Fatal(err)
	}
	path, err := store.ProvisionSupervisorToken("agent-access")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(filepath.Dir(statePath), "credentials") {
		t.Fatalf("token path = %q, want a file in the protected credential directory", path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "first-secret-token" {
		t.Fatalf("provisioned token = %q, err=%v", got, err)
	}
	if runtime.GOOS == "linux" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("token file mode = %o, want 600", info.Mode().Perm())
		}
	}
	if err := store.Put("agent-access", "replacement-secret-token"); err != nil {
		t.Fatal(err)
	}
	if gotPath, err := store.ProvisionSupervisorToken("agent-access"); err != nil || gotPath != path {
		t.Fatalf("replacement provision = %q, err=%v", gotPath, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "replacement-secret-token" {
		t.Fatalf("replaced token = %q, err=%v", got, err)
	}
}

func TestProvisionTokenFileRejectsUnsafeNameAndMissingCredential(t *testing.T) {
	if runtime.GOOS == "windows" && !canPersistPlatformCredentialForTest() {
		t.Skip("Windows Supervisor token-file integration requires an elevated process")
	}
	store := New(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.ProvisionSupervisorToken("missing"); err == nil {
		t.Fatal("missing credential was accepted")
	}
}

func TestDeleteTokenFileIsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" && !canPersistPlatformCredentialForTest() {
		t.Skip("Windows Supervisor token-file integration requires an elevated process")
	}
	store := New(filepath.Join(t.TempDir(), "state.db"))
	if err := store.Put("agent-access", "secret"); err != nil {
		t.Fatal(err)
	}
	path, err := store.ProvisionSupervisorToken("agent-access")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSupervisorToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("token file remains after deletion: %v", err)
	}
	if err := store.DeleteSupervisorToken(); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestOTLPTokenFileIsProtectedAndSeparateFromSupervisor(t *testing.T) {
	if runtime.GOOS == "windows" && !canPersistPlatformCredentialForTest() {
		t.Skip("Windows protected token-file integration requires an elevated process")
	}
	store := New(filepath.Join(t.TempDir(), "state.db"))
	if err := store.Put("fleet/opamp", "opamp-secret"); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("fleet/otlp", "otlp-secret"); err != nil {
		t.Fatal(err)
	}
	supervisorPath, err := store.ProvisionSupervisorToken("fleet/opamp")
	if err != nil {
		t.Fatal(err)
	}
	otlpPath, err := store.ProvisionOTLPToken("fleet/otlp")
	if err != nil {
		t.Fatal(err)
	}
	if otlpPath == supervisorPath {
		t.Fatal("OTLP and Supervisor token paths must be separate")
	}
	if got, err := os.ReadFile(otlpPath); err != nil || string(got) != "otlp-secret" {
		t.Fatalf("OTLP token content invalid: %v", err)
	}
	if runtime.GOOS == "linux" {
		info, err := os.Stat(otlpPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("OTLP token mode = %o, want 600", info.Mode().Perm())
		}
	}
	if err := store.DeleteOTLPToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(otlpPath); !os.IsNotExist(err) {
		t.Fatalf("OTLP token file remains after delete: %v", err)
	}
	if _, err := os.Stat(supervisorPath); err != nil {
		t.Fatalf("Supervisor token was affected by OTLP cleanup: %v", err)
	}
	if err := store.DeleteOTLPToken(); err != nil {
		t.Fatalf("repeat OTLP cleanup: %v", err)
	}
	if _, err := store.ProvisionOTLPToken("missing"); err == nil {
		t.Fatal("missing OTLP credential was accepted")
	}
}

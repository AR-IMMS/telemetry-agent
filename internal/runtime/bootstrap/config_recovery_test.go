package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommitAndRestoreLastKnownGoodConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "otel.yaml")
	if err := os.WriteFile(configPath, []byte("candidate: true\n"), 0o640); err != nil {
		t.Fatalf("write candidate config: %v", err)
	}
	if err := CommitLastKnownGoodConfig(configPath); err != nil {
		t.Fatalf("CommitLastKnownGoodConfig() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("broken: true\n"), 0o640); err != nil {
		t.Fatalf("replace active config: %v", err)
	}
	if err := RestoreLastKnownGoodConfig(configPath); err != nil {
		t.Fatalf("RestoreLastKnownGoodConfig() error = %v", err)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if string(got) != "candidate: true\n" {
		t.Fatalf("restored config = %q, want candidate config", got)
	}
}

func TestRuntimeSnapshotKeepsLastKnownGoodBoundToLaunchedBytes(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "otel.yaml")
	if err := os.WriteFile(configPath, []byte("generation-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	launchPath, cleanup, err := SnapshotRuntimeConfig(configPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := os.WriteFile(configPath, []byte("generation-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CommitLastKnownGoodConfigFrom(configPath, launchPath); err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(configPath + lastKnownGoodSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) != "generation-one" {
		t.Fatalf("last-known-good = %q, want launch snapshot generation-one", committed)
	}
}

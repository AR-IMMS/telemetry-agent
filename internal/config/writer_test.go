package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicCreatesNewFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "config", "otel.yaml")

	if err := WriteAtomic(target, []byte("receivers: {}"), 0640); err != nil {
		t.Fatalf("WriteAtomic() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != "receivers: {}" {
		t.Fatalf("file content = %q, want %q", got, "receivers: {}")
	}
}

func TestWriteAtomicReplacesExistingFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "otel.yaml")

	if err := os.WriteFile(target, []byte("old config"), 0640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := WriteAtomic(target, []byte("new config"), 0640); err != nil {
		t.Fatalf("WriteAtomic() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != "new config" {
		t.Fatalf("file content = %q, want %q", got, "new config")
	}
}

package agentinstallation

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyAgentBinaryCopiesExecutableToNestedDestination(
	t *testing.T,
) {
	sourcePath := filepath.Join(t.TempDir(), "agentctl")
	content := []byte("test Agent binary")

	if err := os.WriteFile(sourcePath, content, 0o751); err != nil {
		t.Fatalf("write source binary: %v", err)
	}
	if err := os.Chmod(sourcePath, 0o751); err != nil {
		t.Fatalf("chmod source binary: %v", err)
	}

	destinationPath := filepath.Join(
		t.TempDir(),
		"nested",
		"installation",
		"agentctl",
	)

	if err := CopyAgentBinary(
		context.Background(),
		sourcePath,
		destinationPath,
	); err != nil {
		t.Fatalf("CopyAgentBinary() error = %v", err)
	}

	gotContent, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("read copied binary: %v", err)
	}
	if string(gotContent) != string(content) {
		t.Fatalf("copied content = %q, want %q", gotContent, content)
	}

	info, err := os.Stat(destinationPath)
	if err != nil {
		t.Fatalf("stat copied binary: %v", err)
	}
	if runtime.GOOS != "windows" {
		if got, want := info.Mode().Perm(),
			os.FileMode(0o751); got != want {
			t.Fatalf("copied mode = %o, want %o", got, want)
		}
	}
}

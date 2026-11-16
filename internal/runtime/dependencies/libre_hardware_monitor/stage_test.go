package librehardwaremonitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"runtime"
	"testing"
)

func TestStageArchiveCreatesVerifiedPrivateFile(t *testing.T) {
	content := []byte("verified Libre Hardware Monitor ZIP bytes")
	checksum := sha256.Sum256(content)

	archivePath, cleanup, err := StageArchive(
		context.Background(),
		Artifact{
			FileName: "LibreHardwareMonitor.zip",
			URL:      "https://example.test/LibreHardwareMonitor.zip",
			SHA256:   hex.EncodeToString(checksum[:]),
		},
		testDownloader(func(
			ctx context.Context,
			url string,
			writer io.Writer,
		) error {
			_, writeErr := writer.Write(content)

			return writeErr
		}),
	)
	if err != nil {
		t.Fatalf("StageArchive() error = %v", err)
	}
	if cleanup == nil {
		t.Fatal("StageArchive() cleanup = nil")
	}
	defer cleanup()

	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf(
			"staged archive permissions = %04o, want 0600",
			info.Mode().Perm(),
		)
	}

	got, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("staged archive = %q, want verified bytes", got)
	}

	cleanup()

	if _, err := os.Stat(archivePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged archive still exists after cleanup: %v", err)
	}
}

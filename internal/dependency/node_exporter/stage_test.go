package nodeexporter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestStageArchiveCreatesVerifiedPrivateFile(t *testing.T) {
	content := []byte("verified Node Exporter archive bytes")
	artifact := PinnedArtifact()
	artifact.SHA256 = sha256Hex(content)

	archivePath, cleanup, err := StageArchive(
		context.Background(),
		artifact,
		downloaderFunc(func(
			_ context.Context,
			_ string,
			destination io.Writer,
		) error {
			_, err := destination.Write(content)

			return err
		}),
	)
	if err != nil {
		t.Fatalf("StageArchive() error = %v", err)
	}
	defer cleanup()

	if filepath.Base(archivePath) != artifact.ArchiveName {
		t.Fatalf(
			"archive path = %q, want file name %q",
			archivePath,
			artifact.ArchiveName,
		)
	}

	got, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != string(content) {
		t.Fatalf(
			"staged archive content = %q, want verified bytes",
			got,
		)
	}
}

func sha256Hex(content []byte) string {
	hash := sha256.Sum256(content)

	return hex.EncodeToString(hash[:])
}

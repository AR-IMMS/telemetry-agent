package nodeexporter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractBinaryExtractsPinnedNodeExporter(t *testing.T) {
	artifact := PinnedArtifact()
	archivePath := writeTarGz(
		t,
		map[string][]byte{
			artifact.BinaryPath: []byte("node-exporter-binary"),
		},
	)
	destinationDir := filepath.Join(t.TempDir(), "installation")

	binaryPath, err := ExtractBinary(
		archivePath,
		artifact,
		destinationDir,
	)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}

	wantPath := filepath.Join(destinationDir, "node_exporter")
	if binaryPath != wantPath {
		t.Fatalf(
			"binary path = %q, want %q",
			binaryPath,
			wantPath,
		)
	}

	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(content, []byte("node-exporter-binary")) {
		t.Fatalf(
			"extracted content = %q, want Node Exporter binary bytes",
			content,
		)
	}
}

func writeTarGz(
	t *testing.T,
	entries map[string][]byte,
) string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "node-exporter.tar.gz")

	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	gzipWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzipWriter)

	for name, content := range entries {
		header := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("archive Close() error = %v", err)
	}

	return archivePath
}

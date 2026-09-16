package nodeexporter

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallArchiveAtomicallyPublishesNodeExporterBinary(
	t *testing.T,
) {
	artifact := PinnedArtifact()
	archivePath := writeTarGz(
		t,
		map[string][]byte{
			artifact.BinaryPath: []byte("node-exporter-binary"),
		},
	)

	installDir := filepath.Join(
		t.TempDir(),
		"node-exporter",
	)

	binaryPath, err := InstallArchive(
		archivePath,
		artifact,
		installDir,
	)
	if err != nil {
		t.Fatalf("InstallArchive() error = %v", err)
	}

	wantPath := filepath.Join(installDir, "node_exporter")
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
			"installed binary content = %q, want expected bytes",
			content,
		)
	}
}

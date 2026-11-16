package librehardwaremonitor

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallArchivePublishesCompleteLibreHardwareMonitorDirectory(
	t *testing.T,
) {
	archivePath := writeZIP(
		t,
		map[string][]byte{
			"LibreHardwareMonitor.exe": {
				'l', 'h', 'm', '-', 'e', 'x', 'e',
			},
			"LibreHardwareMonitorLib.dll": {
				'l', 'h', 'm', '-', 'l', 'i', 'b',
			},
			"de/readme.txt": {
				'l', 'o', 'c', 'a', 'l', 'e',
			},
		},
	)

	installDir := filepath.Join(
		t.TempDir(),
		"LibreHardwareMonitor",
	)

	if err := InstallArchive(archivePath, installDir); err != nil {
		t.Fatalf("InstallArchive() error = %v", err)
	}

	for path, want := range map[string][]byte{
		"LibreHardwareMonitor.exe":    []byte("lhm-exe"),
		"LibreHardwareMonitorLib.dll": []byte("lhm-lib"),
		"de/readme.txt":               []byte("locale"),
	} {
		got, err := os.ReadFile(filepath.Join(installDir, path))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("installed file %q = %q, want %q", path, got, want)
		}
	}
}

// writeZIP creates a test ZIP whose entries use the upstream release layout.
func writeZIP(t *testing.T, files map[string][]byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "LibreHardwareMonitor.zip")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)

	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestInstallArchiveRejectsZipSlipEntry(t *testing.T) {
	archivePath := writeZIP(
		t,
		map[string][]byte{
			"../escaped.txt": []byte("must not escape staging"),
		},
	)

	rootDir := t.TempDir()
	installDir := filepath.Join(rootDir, "LibreHardwareMonitor")

	err := InstallArchive(archivePath, installDir)

	if err == nil {
		t.Fatal("InstallArchive() error = nil, want unsafe-entry error")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf(
			"InstallArchive() error = %v, want unsafe-entry error",
			err,
		)
	}

	escapedPath := filepath.Join(rootDir, "escaped.txt")

	if _, err := os.Stat(escapedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Zip Slip created %q: %v", escapedPath, err)
	}
}

package nodeexporter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemdUnitExistsReportsMissingUnit(t *testing.T) {
	exists, err := systemdUnitExists(
		filepath.Join(t.TempDir(), "missing.service"),
	)
	if err != nil {
		t.Fatalf("systemdUnitExists() error = %v", err)
	}
	if exists {
		t.Fatal("systemdUnitExists() = true, want false")
	}
}

func TestSystemdUnitExistsReportsExistingUnit(t *testing.T) {
	unitPath := filepath.Join(
		t.TempDir(),
		"ar-imms-node-exporter.service",
	)

	if err := os.WriteFile(unitPath, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	exists, err := systemdUnitExists(unitPath)
	if err != nil {
		t.Fatalf("systemdUnitExists() error = %v", err)
	}
	if !exists {
		t.Fatal("systemdUnitExists() = false, want true")
	}
}

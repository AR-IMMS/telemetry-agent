package nodeexporter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallArchive extracts a verified archive into a sibling staging directory
// and atomically publishes the complete installation with a directory rename.
func InstallArchive(
	archivePath string,
	artifact Artifact,
	installDir string,
) (string, error) {
	if strings.TrimSpace(installDir) == "" {
		return "", fmt.Errorf(
			"Node Exporter installation directory is required",
		)
	}

	parentDir := filepath.Dir(installDir)
	installName := filepath.Base(installDir)

	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return "", fmt.Errorf(
			"create Node Exporter installation parent directory: %w",
			err,
		)
	}

	stagingDir, err := os.MkdirTemp(
		parentDir,
		"."+installName+"-",
	)
	if err != nil {
		return "", fmt.Errorf(
			"create Node Exporter installation staging directory: %w",
			err,
		)
	}
	defer os.RemoveAll(stagingDir)

	binaryPath, err := ExtractBinary(
		archivePath,
		artifact,
		stagingDir,
	)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(installDir); err == nil {
		return "", fmt.Errorf(
			"Node Exporter installation target %q already exists",
			installDir,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf(
			"inspect Node Exporter installation target: %w",
			err,
		)
	}

	if err := os.Rename(stagingDir, installDir); err != nil {
		return "", fmt.Errorf(
			"atomically install Node Exporter: %w",
			err,
		)
	}

	return filepath.Join(
		installDir,
		filepath.Base(binaryPath),
	), nil
}

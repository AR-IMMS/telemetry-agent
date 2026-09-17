package librehardwaremonitor

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// InstallArchive extracts the upstream ZIP into a sibling staging directory
// and atomically publishes the complete LHM installation.
func InstallArchive(
	archivePath string,
	installDir string,
) error {
	if strings.TrimSpace(archivePath) == "" {
		return fmt.Errorf("Libre Hardware Monitor archive path is required")
	}
	if strings.TrimSpace(installDir) == "" {
		return fmt.Errorf(
			"Libre Hardware Monitor installation directory is required",
		)
	}

	parentDir := filepath.Dir(installDir)
	installName := filepath.Base(installDir)

	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf(
			"create Libre Hardware Monitor installation parent directory: %w",
			err,
		)
	}

	stagingDir, err := os.MkdirTemp(parentDir, "."+installName+"-")
	if err != nil {
		return fmt.Errorf(
			"create Libre Hardware Monitor installation staging directory: %w",
			err,
		)
	}
	defer os.RemoveAll(stagingDir)

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open Libre Hardware Monitor archive: %w", err)
	}
	defer archive.Close()

	for _, entry := range archive.File {
		if err := extractArchiveEntry(entry, stagingDir); err != nil {
			return err
		}
	}

	if _, err := os.Stat(installDir); err == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor installation target %q already exists",
			installDir,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"inspect Libre Hardware Monitor installation target: %w",
			err,
		)
	}

	if err := os.Rename(stagingDir, installDir); err != nil {
		return fmt.Errorf(
			"atomically install Libre Hardware Monitor: %w",
			err,
		)
	}

	return nil
}

// extractArchiveEntry writes one ZIP-root entry below stagingDir and prevents
// any archive path from escaping the staging directory.
func extractArchiveEntry(
	entry *zip.File,
	stagingDir string,
) error {
	name := path.Clean(entry.Name)

	if name == "." {
		return nil
	}
	if path.IsAbs(name) ||
		name == ".." ||
		strings.HasPrefix(name, "../") {
		return fmt.Errorf(
			"Libre Hardware Monitor archive entry %q is unsafe",
			entry.Name,
		)
	}

	relativePath := filepath.FromSlash(name)

	if filepath.IsAbs(relativePath) {
		return fmt.Errorf(
			"Libre Hardware Monitor archive entry %q is unsafe",
			entry.Name,
		)
	}

	destination := filepath.Join(stagingDir, relativePath)

	relativeDestination, err := filepath.Rel(stagingDir, destination)
	if err != nil ||
		relativeDestination == ".." ||
		strings.HasPrefix(
			relativeDestination,
			".."+string(os.PathSeparator),
		) {
		return fmt.Errorf(
			"Libre Hardware Monitor archive entry %q is unsafe",
			entry.Name,
		)
	}

	if entry.FileInfo().IsDir() {
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return fmt.Errorf(
				"create Libre Hardware Monitor archive directory: %w",
				err,
			)
		}

		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf(
			"create Libre Hardware Monitor archive file directory: %w",
			err,
		)
	}

	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf(
			"open Libre Hardware Monitor archive entry %q: %w",
			entry.Name,
			err,
		)
	}
	defer source.Close()

	destinationFile, err := os.OpenFile(
		destination,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o755,
	)
	if err != nil {
		return fmt.Errorf(
			"create Libre Hardware Monitor archive file %q: %w",
			entry.Name,
			err,
		)
	}

	_, copyErr := io.Copy(destinationFile, source)
	closeErr := destinationFile.Close()

	if copyErr != nil {
		return fmt.Errorf(
			"extract Libre Hardware Monitor archive entry %q: %w",
			entry.Name,
			copyErr,
		)
	}
	if closeErr != nil {
		return fmt.Errorf(
			"close Libre Hardware Monitor archive file %q: %w",
			entry.Name,
			closeErr,
		)
	}

	return nil
}

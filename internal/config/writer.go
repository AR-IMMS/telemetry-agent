package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteAtomic writes content to path without exposing a partially written file.
//
// The new content is written to a temporary file in the target directory and
// replaces the target only after the write, sync, and close operations succeed.
func WriteAtomic(path string, content []byte, permission fs.FileMode) error {
	if path == "" {
		return fmt.Errorf("write configuration: path is required")
	}

	directory := filepath.Dir(path)

	if err := os.MkdirAll(directory, 0775); err != nil {
		return fmt.Errorf(
			"create configuration directory %q: %w",
			directory,
			err,
		)
	}

	tempFile, err := os.CreateTemp(
		directory,
		"."+filepath.Base(path)+".",
	)

	if err != nil {
		return fmt.Errorf(
			"create temporary configuration file: %w",
			err,
		)
	}

	tempPath := tempFile.Name()

	// Removing an already renamed path is harmless. This also cleans up any
	// temporary file left behind when writing or renaming fails.
	defer os.Remove(tempPath)

	if err := tempFile.Chmod(permission); err != nil {
		tempFile.Close()
		return fmt.Errorf(
			"set temporary configuration permissions: %w",
			err,
		)
	}

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf(
			"write temporary configuration file: %w",
			err,
		)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf(
			"sync temporary configuration file: %w",
			err,
		)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf(
			"close temporary configuration file: %w",
			err,
		)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf(
			"rename temporary configuration file: %w",
			err,
		)
	}

	return nil
}

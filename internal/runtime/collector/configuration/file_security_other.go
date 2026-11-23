//go:build !windows

package collectorconfig

import (
	"errors"
	"fmt"
	"os"
)

// PreserveFileSecurity copies the existing regular file's permission bits to a staged replacement.
// A missing source is allowed for first-time configuration creation.
func PreserveFileSecurity(sourcePath string, stagedPath string) error {
	sourceInfo, err := os.Lstat(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing configuration security: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("existing configuration is not a regular file")
	}
	stagedInfo, err := os.Lstat(stagedPath)
	if err != nil {
		return fmt.Errorf("inspect staged configuration: %w", err)
	}
	if !stagedInfo.Mode().IsRegular() {
		return fmt.Errorf("staged configuration is not a regular file")
	}
	if err := os.Chmod(stagedPath, sourceInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("preserve existing configuration mode: %w", err)
	}
	return nil
}

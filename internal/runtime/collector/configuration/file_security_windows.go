//go:build windows

package collectorconfig

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// PreserveFileSecurity copies the existing regular file's owner, group, and DACL to a staged replacement.
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

	securityInformation := windows.SECURITY_INFORMATION(
		windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION,
	)
	sourceDescriptor, err := windows.GetNamedSecurityInfo(sourcePath, windows.SE_FILE_OBJECT, securityInformation)
	if err != nil {
		return fmt.Errorf("read existing configuration security descriptor: %w", err)
	}
	owner, _, err := sourceDescriptor.Owner()
	if err != nil {
		return fmt.Errorf("read existing configuration owner: %w", err)
	}
	group, _, err := sourceDescriptor.Group()
	if err != nil {
		return fmt.Errorf("read existing configuration group: %w", err)
	}
	dacl, _, err := sourceDescriptor.DACL()
	if err != nil {
		return fmt.Errorf("read existing configuration DACL: %w", err)
	}
	control, _, err := sourceDescriptor.Control()
	if err != nil {
		return fmt.Errorf("read existing configuration DACL inheritance state: %w", err)
	}
	if control&windows.SE_DACL_PROTECTED != 0 {
		securityInformation |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		securityInformation |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetNamedSecurityInfo(stagedPath, windows.SE_FILE_OBJECT, securityInformation, owner, group, dacl, nil); err != nil {
		return fmt.Errorf("apply existing configuration security descriptor to staged file: %w", err)
	}
	return nil
}

//go:build linux

package identity

import (
	"os"
	"strings"
)

func collectPlatformDetails() (platformDetails, error) {
	// Kernel release is required; distribution metadata is optional on Linux hosts.
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return platformDetails{}, err
	}
	details := platformDetails{Kernel: "Linux", Release: strings.TrimSpace(string(release))}
	if content, err := os.ReadFile("/etc/os-release"); err == nil {
		parsed := parseOSRelease(content)
		details.Distribution, details.Version = parsed.Distribution, parsed.Version
	}
	return details, nil
}

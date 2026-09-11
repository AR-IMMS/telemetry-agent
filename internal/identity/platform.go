package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

// PlatformInfo identifies the host runtime and available operating-system details.
type PlatformInfo struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Hostname     string `json:"hostname,omitempty"`
	Kernel       string `json:"kernel,omitempty"`
	Release      string `json:"release,omitempty"`
	Distribution string `json:"distribution,omitempty"`
	Version      string `json:"version,omitempty"`
}

// CollectPlatformInfo reads runtime, hostname, and platform-specific host details.
func CollectPlatformInfo() (PlatformInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return PlatformInfo{}, fmt.Errorf("resolve hostname: %w", err)
	}
	info := PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH, Hostname: hostname}
	details, err := collectPlatformDetails()
	if err != nil {
		return PlatformInfo{}, fmt.Errorf("collect %s platform details: %w", runtime.GOOS, err)
	}
	info.Kernel, info.Release = details.Kernel, details.Release
	info.Distribution, info.Version = details.Distribution, details.Version
	return info, nil
}

// AsJSON serializes platform information using its stable JSON field names.
func (p PlatformInfo) AsJSON() ([]byte, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal platform info: %w", err)
	}
	return payload, nil
}

type platformDetails struct {
	Kernel, Release, Distribution, Version string
}

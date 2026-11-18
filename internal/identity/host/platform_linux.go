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

func collectEnvironmentPlatformDetails() (platformDetails, error) {
	return collectPlatformDetails()
}

func readPlatformEnvironmentSignals() platformEnvironmentSignals {
	var manufacturer, product string
	if value, err := os.ReadFile("/sys/class/dmi/id/sys_vendor"); err == nil {
		manufacturer = string(value)
	}
	if value, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
		product = string(value)
	}
	virtualization, physical := normalizeHardwareEvidence(manufacturer, product)
	container := pathExists("/.dockerenv") || pathExists("/run/.containerenv")
	if content, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		lower := strings.ToLower(string(content))
		for _, marker := range []string{"docker", "containerd", "kubepods", "libpod", "lxc"} {
			if strings.Contains(lower, marker) {
				container = true
				break
			}
		}
	}
	environment := detectKubernetesSignals(map[string]string{
		"KUBERNETES_SERVICE_HOST": os.Getenv("KUBERNETES_SERVICE_HOST"),
		"KUBERNETES_SERVICE_PORT": os.Getenv("KUBERNETES_SERVICE_PORT"),
		"POD_UID":                 os.Getenv("POD_UID"),
		"AR_IMMS_ORCHESTRATOR":    os.Getenv("AR_IMMS_ORCHESTRATOR"),
	})
	environment.VirtualizationVendor = virtualization
	environment.PhysicalHardwareHint = physical
	environment.ContainerRuntimeDetected = container
	return environment
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

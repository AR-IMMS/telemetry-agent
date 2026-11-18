//go:build windows

package identity

import (
	"os"
	"runtime"

	"golang.org/x/sys/windows/registry"
)

func collectPlatformDetails() (platformDetails, error) {
	return platformDetails{Kernel: runtime.GOOS}, nil
}

func collectEnvironmentPlatformDetails() (platformDetails, error) {
	details := platformDetails{Kernel: runtime.GOOS}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return details, nil
	}
	defer key.Close()
	product, _, _ := key.GetStringValue("ProductName")
	release, _, _ := key.GetStringValue("DisplayVersion")
	build, _, _ := key.GetStringValue("CurrentBuildNumber")
	details.Version = joinVersionParts(product, release, build)
	return details, nil
}

func readPlatformEnvironmentSignals() platformEnvironmentSignals {
	var manufacturer, product string
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err == nil {
		manufacturer, _, _ = key.GetStringValue("SystemManufacturer")
		product, _, _ = key.GetStringValue("SystemProductName")
		_ = key.Close()
	}
	virtualization, physical := normalizeHardwareEvidence(manufacturer, product)
	environment := detectKubernetesSignals(map[string]string{
		"KUBERNETES_SERVICE_HOST":  os.Getenv("KUBERNETES_SERVICE_HOST"),
		"KUBERNETES_SERVICE_PORT":  os.Getenv("KUBERNETES_SERVICE_PORT"),
		"POD_UID":                  os.Getenv("POD_UID"),
		"AR_IMMS_DEPLOYMENT_SCOPE": os.Getenv("AR_IMMS_DEPLOYMENT_SCOPE"),
		"AR_IMMS_ORCHESTRATOR":     os.Getenv("AR_IMMS_ORCHESTRATOR"),
	})
	environment.VirtualizationVendor = virtualization
	environment.PhysicalHardwareHint = physical
	environment.ContainerRuntimeDetected = os.Getenv("CONTAINER_SANDBOX_MOUNT_POINT") != "" || os.Getenv("CONTAINER_ID") != ""
	return environment
}

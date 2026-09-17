package librehardwaremonitor

import (
	"fmt"
	"strings"
)

// Options identifies every Windows resource owned by the LHM dependency.
type Options struct {
	// InstallDir contains LibreHardwareMonitor.exe and its managed config.
	InstallDir string

	// TaskName is the LocalSystem Scheduled Task that runs LHM at startup.
	TaskName string

	// ListenPort is the local HTTP metrics port used by the OTel Agent.
	ListenPort int

	// FirewallName is the managed inbound-block firewall rule.
	FirewallName string
}

// Validate rejects incomplete options before changing Windows resources.
func (o Options) Validate() error {
	if strings.TrimSpace(o.InstallDir) == "" {
		return fmt.Errorf(
			"Libre Hardware Monitor installation directory is required",
		)
	}
	if strings.TrimSpace(o.TaskName) == "" {
		return fmt.Errorf(
			"Libre Hardware Monitor task name is required",
		)
	}
	if o.ListenPort <= 0 || o.ListenPort > 65535 {
		return fmt.Errorf(
			"Libre Hardware Monitor listen port must be between 1 and 65535",
		)
	}
	if strings.TrimSpace(o.FirewallName) == "" {
		return fmt.Errorf(
			"Libre Hardware Monitor firewall rule name is required",
		)
	}

	return nil
}

// DefaultOptions returns the stable Windows resources owned by this Agent.
func DefaultOptions() Options {
	return Options{
		InstallDir:   `C:\Program Files\AR-IMMS\LibreHardwareMonitor`,
		TaskName:     "AR-IMMS-LibreHardwareMonitor",
		ListenPort:   9190,
		FirewallName: "AR-IMMS Libre Hardware Monitor metrics",
	}
}

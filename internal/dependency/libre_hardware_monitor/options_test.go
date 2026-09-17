package librehardwaremonitor

import (
	"strings"
	"testing"
)

func TestOptionsValidateRejectsMissingRequiredValues(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{
			name: "missing installation directory",
			options: Options{
				TaskName:     "AR-IMMS-LibreHardwareMonitor",
				ListenPort:   9190,
				FirewallName: "AR-IMMS Libre Hardware Monitor metrics",
			},
			want: "installation directory",
		},
		{
			name: "missing task name",
			options: Options{
				InstallDir:   `C:\Program Files\AR-IMMS\LibreHardwareMonitor`,
				ListenPort:   9190,
				FirewallName: "AR-IMMS Libre Hardware Monitor metrics",
			},
			want: "task name",
		},
		{
			name: "missing listen port",
			options: Options{
				InstallDir:   `C:\Program Files\AR-IMMS\LibreHardwareMonitor`,
				TaskName:     "AR-IMMS-LibreHardwareMonitor",
				FirewallName: "AR-IMMS Libre Hardware Monitor metrics",
			},
			want: "listen port",
		},
		{
			name: "missing firewall name",
			options: Options{
				InstallDir: `C:\Program Files\AR-IMMS\LibreHardwareMonitor`,
				TaskName:   "AR-IMMS-LibreHardwareMonitor",
				ListenPort: 9190,
			},
			want: "firewall rule name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.options.Validate()

			if err == nil {
				t.Fatal("Options.Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf(
					"Options.Validate() error = %q, want text containing %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestDefaultOptionsUseAgentOwnedWindowsResources(t *testing.T) {
	options := DefaultOptions()

	if options.InstallDir != `C:\Program Files\AR-IMMS\LibreHardwareMonitor` {
		t.Fatalf(
			"InstallDir = %q, want Agent-owned Program Files directory",
			options.InstallDir,
		)
	}
	if options.TaskName != "AR-IMMS-LibreHardwareMonitor" {
		t.Fatalf(
			"TaskName = %q, want stable Agent-owned task name",
			options.TaskName,
		)
	}
	if options.ListenPort != 9190 {
		t.Fatalf(
			"ListenPort = %d, want 9190",
			options.ListenPort,
		)
	}
	if options.FirewallName != "AR-IMMS Libre Hardware Monitor metrics" {
		t.Fatalf(
			"FirewallName = %q, want stable managed rule name",
			options.FirewallName,
		)
	}

	if err := options.Validate(); err != nil {
		t.Fatalf("DefaultOptions().Validate() error = %v", err)
	}
}

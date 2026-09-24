//go:build windows

package dependency

import (
	"testing"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func TestWindowsExporterInstallationStateFromSCM(
	t *testing.T,
) {
	testCases := []struct {
		name          string
		startType     uint32
		serviceState  svc.State
		wantEnabled   bool
		wantRunning   bool
		wantStartupOK bool
	}{
		{
			name:          "automatic running",
			startType:     mgr.StartAutomatic,
			serviceState:  svc.Running,
			wantEnabled:   true,
			wantRunning:   true,
			wantStartupOK: true,
		},
		{
			name:          "manual stopped",
			startType:     mgr.StartManual,
			serviceState:  svc.Stopped,
			wantEnabled:   true,
			wantRunning:   false,
			wantStartupOK: false,
		},
		{
			name:         "disabled stopped",
			startType:    mgr.StartDisabled,
			serviceState: svc.Stopped,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := windowsExporterInstallationStateFromSCM(
				testCase.startType,
				testCase.serviceState,
			)
			if err != nil {
				t.Fatalf(
					"windowsExporterInstallationStateFromSCM() error = %v",
					err,
				)
			}

			if !got.serviceExists {
				t.Fatal("serviceExists = false, want true")
			}
			if got.serviceEnabled != testCase.wantEnabled {
				t.Fatalf(
					"serviceEnabled = %t, want %t",
					got.serviceEnabled,
					testCase.wantEnabled,
				)
			}
			if got.serviceRunning != testCase.wantRunning {
				t.Fatalf(
					"serviceRunning = %t, want %t",
					got.serviceRunning,
					testCase.wantRunning,
				)
			}
			if got.startupMatches != testCase.wantStartupOK {
				t.Fatalf(
					"startupMatches = %t, want %t",
					got.startupMatches,
					testCase.wantStartupOK,
				)
			}
		})
	}
}

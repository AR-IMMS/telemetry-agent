//go:build windows

package dependency

import (
	"testing"

	"golang.org/x/sys/windows/svc/mgr"
)

func TestWindowsExporterStartupStateClassifiesServiceStartType(
	t *testing.T,
) {
	testCases := []struct {
		name        string
		startType   uint32
		wantEnabled bool
		wantMatches bool
		wantError   bool
	}{
		{
			name:        "automatic",
			startType:   mgr.StartAutomatic,
			wantEnabled: true,
			wantMatches: true,
		},
		{
			name:        "manual",
			startType:   mgr.StartManual,
			wantEnabled: true,
			wantMatches: false,
		},
		{
			name:      "disabled",
			startType: mgr.StartDisabled,
		},
		{
			name:      "unknown",
			startType: 99,
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			enabled, matches, err := windowsExporterStartupState(
				testCase.startType,
			)

			if testCase.wantError {
				if err == nil {
					t.Fatal("windowsExporterStartupState() error = nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("windowsExporterStartupState() error = %v", err)
			}
			if enabled != testCase.wantEnabled {
				t.Fatalf(
					"enabled = %t, want %t",
					enabled,
					testCase.wantEnabled,
				)
			}
			if matches != testCase.wantMatches {
				t.Fatalf(
					"matches = %t, want %t",
					matches,
					testCase.wantMatches,
				)
			}
		})
	}
}

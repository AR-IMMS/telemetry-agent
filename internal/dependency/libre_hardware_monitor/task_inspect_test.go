package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"
)

func TestScheduledTaskExistsReadsPowerShellTrueResult(t *testing.T) {
	exists, err := scheduledTaskExists(
		context.Background(),
		"AR-IMMS-LibreHardwareMonitor",
		func(
			ctx context.Context,
			command processCommand,
		) ([]byte, error) {
			if command.Executable != "powershell.exe" {
				t.Fatalf(
					"Executable = %q, want powershell.exe",
					command.Executable,
				)
			}

			script := command.Args[len(command.Args)-1]

			for _, want := range []string{
				"Get-ScheduledTask",
				"AR-IMMS-LibreHardwareMonitor",
			} {
				if !strings.Contains(script, want) {
					t.Fatalf(
						"task query script does not contain %q:\n%s",
						want,
						script,
					)
				}
			}

			return []byte("true\n"), nil
		},
	)
	if err != nil {
		t.Fatalf("scheduledTaskExists() error = %v", err)
	}
	if !exists {
		t.Fatal("scheduledTaskExists() = false, want true")
	}
}

func TestScheduledTaskMatchesRecognizesManagedDefinition(t *testing.T) {
	options := DefaultOptions()

	expectedXML, err := RenderTaskXML(
		options,
		testInteractiveUserSID,
	)
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	expectedXMLText := decodeUTF16LE(t, expectedXML)

	matches, err := scheduledTaskMatches(
		context.Background(),
		options,
		testInteractiveUserSID,
		func(
			ctx context.Context,
			command processCommand,
		) ([]byte, error) {
			script := command.Args[len(command.Args)-1]

			for _, want := range []string{
				"[Console]::OutputEncoding",
				"UTF8Encoding",
				"Export-ScheduledTask",
			} {
				if !strings.Contains(script, want) {
					t.Fatalf(
						"task matching script does not contain %q:\n%s",
						want,
						script,
					)
				}
			}

			// PowerShell query returns XML text; it does not return the
			// UTF-16LE task-definition file bytes used by schtasks.exe.
			return []byte(expectedXMLText), nil
		},
	)
	if err != nil {
		t.Fatalf("scheduledTaskMatches() error = %v", err)
	}
	if !matches {
		t.Fatal("scheduledTaskMatches() = false, want true")
	}
}

func TestScheduledTaskEnabledReadsPowerShellTrueResult(t *testing.T) {
	enabled, err := scheduledTaskEnabled(
		context.Background(),
		"AR-IMMS-LibreHardwareMonitor",
		func(
			ctx context.Context,
			command processCommand,
		) ([]byte, error) {
			if command.Executable != "powershell.exe" {
				t.Fatalf(
					"Executable = %q, want powershell.exe",
					command.Executable,
				)
			}

			script := command.Args[len(command.Args)-1]

			for _, want := range []string{
				"Get-ScheduledTask",
				"AR-IMMS-LibreHardwareMonitor",
				"Disabled",
			} {
				if !strings.Contains(script, want) {
					t.Fatalf(
						"task enabled script does not contain %q:\n%s",
						want,
						script,
					)
				}
			}

			return []byte("true\n"), nil
		},
	)
	if err != nil {
		t.Fatalf("scheduledTaskEnabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("scheduledTaskEnabled() = false, want true")
	}
}

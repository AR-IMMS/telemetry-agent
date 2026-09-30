package agentinstallation

import "testing"

func TestWindowsServiceDefinitionCommandLineQuotesPaths(t *testing.T) {
	definition := windowsServiceDefinition{
		ExecutablePath: `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
		Args: []string{
			"run",
			"--state-path",
			`C:\ProgramData\AR-IMMS\Telemetry Agent\state.json`,
		},
	}

	got := definition.commandLine()

	want := `"C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe" run --state-path "C:\ProgramData\AR-IMMS\Telemetry Agent\state.json"`

	if got != want {
		t.Fatalf("command line = %q, want %q", got, want)
	}
}

package librehardwaremonitor

import (
	"context"
	"reflect"
	"testing"
)

func TestNewPowerShellScriptRunnerUsesNonInteractivePowerShell(
	t *testing.T,
) {
	var gotCommand processCommand

	runScript := newPowerShellScriptRunner(
		func(
			ctx context.Context,
			command processCommand,
		) error {
			gotCommand = command
			return nil
		},
	)

	script := "Write-Output 'test'"

	if err := runScript(context.Background(), script); err != nil {
		t.Fatalf("PowerShell runner error = %v", err)
	}

	wantCommand := processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			script,
		},
	}

	if !reflect.DeepEqual(gotCommand, wantCommand) {
		t.Fatalf(
			"PowerShell command = %#v, want %#v",
			gotCommand,
			wantCommand,
		)
	}
}

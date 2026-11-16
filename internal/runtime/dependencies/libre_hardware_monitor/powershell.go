package librehardwaremonitor

import (
	"context"
	"fmt"
	"strings"
)

// newPowerShellScriptRunner adapts the generic process runner to PowerShell.
func newPowerShellScriptRunner(
	run processRunner,
) powerShellScriptRunner {
	return func(ctx context.Context, script string) error {
		if run == nil {
			return fmt.Errorf(
				"Libre Hardware Monitor process runner is required",
			)
		}
		if strings.TrimSpace(script) == "" {
			return fmt.Errorf(
				"Libre Hardware Monitor PowerShell script is required",
			)
		}

		return run(ctx, processCommand{
			Executable: "powershell.exe",
			Args: []string{
				"-NoProfile",
				"-NonInteractive",
				"-Command",
				script,
			},
		})
	}
}

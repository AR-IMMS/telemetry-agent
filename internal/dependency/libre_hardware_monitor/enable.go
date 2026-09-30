package librehardwaremonitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// lhmEnabler starts the Agent-owned Libre Hardware Monitor scheduled task.
type lhmEnabler struct {
	options       Options
	administrator administratorProbe
	runProcess    processRunner
}

// Enable restores scheduled-task enablement and starts the owned LHM task.
func (e lhmEnabler) Enable(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if ctx == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor enable context is required",
		)
	}

	if err := e.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Libre Hardware Monitor enable options: %w",
			err,
		)
	}
	if err := requireWindowsAdministrator(e.administrator); err != nil {
		return err
	}
	if e.runProcess == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor enable process runner is required",
		)
	}
	if !ownsLHMResource(
		resources,
		"scheduled-task",
		e.options.TaskName,
	) {
		return fmt.Errorf(
			"Libre Hardware Monitor enable requires owned scheduled task %q",
			e.options.TaskName,
		)
	}

	taskName := strings.ReplaceAll(e.options.TaskName, "'", "''")

	script := fmt.Sprintf(
		"$task = Get-ScheduledTask -TaskName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"if ($null -eq $task) {\n"+
			"  throw 'Libre Hardware Monitor scheduled task is missing'\n"+
			"}\n"+
			"Enable-ScheduledTask -TaskName '%s' -ErrorAction Stop\n"+
			"if ($task.State -ne 'Running') {\n"+
			"  Start-ScheduledTask -TaskName '%s' -ErrorAction Stop\n"+
			"}\n",
		taskName,
		taskName,
		taskName,
	)

	if err := e.runProcess(ctx, processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			script,
		},
	}); err != nil {
		return fmt.Errorf(
			"enable Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	return nil
}

// NewEnabler creates the production Libre Hardware Monitor enable adapter.
func NewEnabler(options Options) dependency.Enabler {
	return lhmEnabler{
		options:       options,
		administrator: isWindowsAdministrator,
		runProcess:    runOSProcess,
	}.Enable
}

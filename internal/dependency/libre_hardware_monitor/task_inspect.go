package librehardwaremonitor

import (
	"context"
	"fmt"
	"strings"
)

// scheduledTaskExists checks Task Scheduler without treating an absent task
// as a process failure.
func scheduledTaskExists(
	ctx context.Context,
	taskName string,
	run processOutputRunner,
) (bool, error) {
	taskName = strings.TrimSpace(taskName)

	if taskName == "" {
		return false, fmt.Errorf(
			"Libre Hardware Monitor task name is required",
		)
	}
	if run == nil {
		return false, fmt.Errorf(
			"Libre Hardware Monitor process output runner is required",
		)
	}

	escapedTaskName := strings.ReplaceAll(taskName, "'", "''")

	script := fmt.Sprintf(
		"$task = Get-ScheduledTask -TaskName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"if ($null -eq $task) {\n"+
			"  [Console]::Out.Write('false')\n"+
			"} else {\n"+
			"  [Console]::Out.Write('true')\n"+
			"}\n",
		escapedTaskName,
	)

	output, err := run(ctx, processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			script,
		},
	})
	if err != nil {
		return false, fmt.Errorf(
			"query Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	switch strings.ToLower(strings.TrimSpace(string(output))) {
	case "true":
		return true, nil

	case "false":
		return false, nil

	default:
		return false, fmt.Errorf(
			"unexpected Libre Hardware Monitor task query result %q",
			strings.TrimSpace(string(output)),
		)
	}
}

// scheduledTaskMatches exports the task definition and checks its managed
// startup, LocalSystem, restart, and executable settings.
func scheduledTaskMatches(
	ctx context.Context,
	options Options,
	run processOutputRunner,
) (bool, error) {
	if err := options.Validate(); err != nil {
		return false, fmt.Errorf(
			"validate Libre Hardware Monitor task options: %w",
			err,
		)
	}
	if run == nil {
		return false, fmt.Errorf(
			"Libre Hardware Monitor process output runner is required",
		)
	}

	escapedTaskName := strings.ReplaceAll(
		options.TaskName,
		"'",
		"''",
	)

	script := fmt.Sprintf(
		"[Console]::OutputEncoding = "+
			"[System.Text.UTF8Encoding]::new($false)\n"+
			"Export-ScheduledTask -TaskName '%s'\n",
		escapedTaskName,
	)

	output, err := run(ctx, processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			script,
		},
	})
	if err != nil {
		return false, fmt.Errorf(
			"export Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	return taskXMLMatches(options, string(output)), nil
}

// taskXMLMatches checks only the fields owned by the Agent. Task Scheduler may
// add unrelated defaults when it persists the definition.
func taskXMLMatches(
	options Options,
	taskXML string,
) bool {
	executablePath := windowsExecutablePath(options.InstallDir)

	for _, want := range []string{
		"<BootTrigger>",
		"<UserId>S-1-5-18</UserId>",
		"<RunLevel>HighestAvailable</RunLevel>",
		"<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>",
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<RestartOnFailure>",
		"<Interval>PT1M</Interval>",
		"<Count>3</Count>",
		"<Command>" + executablePath + "</Command>",
	} {
		if !strings.Contains(taskXML, want) {
			return false
		}
	}

	return true
}

package librehardwaremonitor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// lhmTeardown applies safe lifecycle actions only to Agent-owned LHM resources.
type lhmTeardown struct {
	options         Options
	administrator   administratorProbe
	runProcess      processRunner
	removeDirectory func(string) error
}

// Teardown disables or uninstalls Agent-owned LHM resources.
func (t lhmTeardown) Teardown(
	ctx context.Context,
	action agentstate.TeardownAction,
	resources []agentstate.OwnedResource,
) error {
	if ctx == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor teardown context is required",
		)
	}
	if err := t.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Libre Hardware Monitor teardown options: %w",
			err,
		)
	}
	if err := requireWindowsAdministrator(t.administrator); err != nil {
		return err
	}
	if t.runProcess == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor teardown process runner is required",
		)
	}

	switch action {
	case agentstate.TeardownActionDisable:
		return t.disableOwnedTask(ctx, resources)

	case agentstate.TeardownActionUninstall:
		return t.uninstallOwnedResources(ctx, resources)

	default:
		return fmt.Errorf(
			"unsupported Libre Hardware Monitor teardown action %q",
			action,
		)
	}
}

func (t lhmTeardown) disableOwnedTask(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if !ownsLHMResource(
		resources,
		"scheduled-task",
		t.options.TaskName,
	) {
		return fmt.Errorf(
			"Libre Hardware Monitor disable requires owned scheduled task %q",
			t.options.TaskName,
		)
	}

	taskName := strings.ReplaceAll(t.options.TaskName, "'", "''")

	script := fmt.Sprintf(
		"$task = Get-ScheduledTask -TaskName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"if ($null -eq $task) {\n"+
			"  exit 0\n"+
			"}\n"+
			"if ($task.State -eq 'Running') {\n"+
			"  Stop-ScheduledTask -TaskName '%s' -ErrorAction Stop\n"+
			"}\n"+
			"Disable-ScheduledTask -TaskName '%s' -ErrorAction Stop\n",
		taskName,
		taskName,
		taskName,
	)

	if err := t.runProcess(ctx, processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			script,
		},
	}); err != nil {
		return fmt.Errorf(
			"disable Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	return nil
}

func (t lhmTeardown) uninstallOwnedResources(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if t.removeDirectory == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor teardown directory remover is required",
		)
	}
	if !ownsLHMResource(
		resources,
		"firewall-rule",
		t.options.FirewallName,
	) {
		return fmt.Errorf(
			"Libre Hardware Monitor uninstall requires owned firewall rule %q",
			t.options.FirewallName,
		)
	}
	if !ownsLHMResource(
		resources,
		"directory",
		t.options.InstallDir,
	) {
		return fmt.Errorf(
			"Libre Hardware Monitor uninstall requires owned installation directory %q",
			t.options.InstallDir,
		)
	}

	if err := t.disableOwnedTask(ctx, resources); err != nil {
		return err
	}

	taskName := strings.ReplaceAll(t.options.TaskName, "'", "''")
	firewallName := strings.ReplaceAll(t.options.FirewallName, "'", "''")

	script := fmt.Sprintf(
		"$task = Get-ScheduledTask -TaskName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"if ($null -ne $task) {\n"+
			"  Unregister-ScheduledTask -TaskName '%s' "+
			"-Confirm:$false -ErrorAction Stop\n"+
			"}\n"+
			"$rules = Get-NetFirewallRule -DisplayName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"if ($null -ne $rules) {\n"+
			"  $rules | Remove-NetFirewallRule -ErrorAction Stop\n"+
			"}\n",
		taskName,
		taskName,
		firewallName,
	)

	if err := t.runProcess(ctx, processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			script,
		},
	}); err != nil {
		return fmt.Errorf(
			"remove Libre Hardware Monitor scheduled task and firewall rule: %w",
			err,
		)
	}

	if err := t.removeDirectory(t.options.InstallDir); err != nil {
		return fmt.Errorf(
			"remove Libre Hardware Monitor installation directory: %w",
			err,
		)
	}

	return nil
}

func ownsLHMResource(
	resources []agentstate.OwnedResource,
	kind string,
	identifier string,
) bool {
	for _, resource := range resources {
		if resource.Kind == kind && resource.Identifier == identifier {
			return true
		}
	}

	return false
}

// NewTeardown creates the production LHM teardown adapter.
func NewTeardown(options Options) dependency.Teardown {
	return lhmTeardown{
		options:         options,
		administrator:   isWindowsAdministrator,
		runProcess:      runOSProcess,
		removeDirectory: os.RemoveAll,
	}.Teardown
}

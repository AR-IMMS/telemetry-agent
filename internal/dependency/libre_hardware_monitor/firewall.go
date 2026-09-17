package librehardwaremonitor

import (
	"context"
	"fmt"
	"strings"
)

// powerShellScriptRunner executes one complete PowerShell script.
type powerShellScriptRunner func(context.Context, string) error

// ConfigureFirewall replaces the Agent-owned rule that blocks remote inbound
// access to the wildcard LHM HTTP listener.
func ConfigureFirewall(
	ctx context.Context,
	options Options,
	run powerShellScriptRunner,
) error {
	if err := options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Libre Hardware Monitor firewall options: %w",
			err,
		)
	}
	if run == nil {
		return fmt.Errorf("Libre Hardware Monitor PowerShell runner is required")
	}

	script := fmt.Sprintf(
		"$rule = Get-NetFirewallRule -DisplayName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"if ($null -ne $rule) {\n"+
			"  $rule | Remove-NetFirewallRule\n"+
			"}\n"+
			"New-NetFirewallRule "+
			"-DisplayName '%s' "+
			"-Direction Inbound "+
			"-Action Block "+
			"-Protocol TCP "+
			"-LocalPort %d "+
			"-Profile Any | Out-Null\n",
		options.FirewallName,
		options.FirewallName,
		options.ListenPort,
	)

	if err := run(ctx, script); err != nil {
		return fmt.Errorf(
			"configure Libre Hardware Monitor firewall: %w",
			err,
		)
	}

	return nil
}

// firewallMatches reports whether a managed inbound-block rule protects the
// wildcard LHM listener on the configured TCP port.
func firewallMatches(
	ctx context.Context,
	options Options,
	run processOutputRunner,
) (bool, error) {
	if err := options.Validate(); err != nil {
		return false, fmt.Errorf(
			"validate Libre Hardware Monitor firewall options: %w",
			err,
		)
	}
	if run == nil {
		return false, fmt.Errorf(
			"Libre Hardware Monitor process output runner is required",
		)
	}

	escapedRuleName := strings.ReplaceAll(
		options.FirewallName,
		"'",
		"''",
	)

	script := fmt.Sprintf(
		"$rules = Get-NetFirewallRule -DisplayName '%s' "+
			"-ErrorAction SilentlyContinue\n"+
			"$matches = $false\n"+
			"foreach ($rule in $rules) {\n"+
			"  $filter = Get-NetFirewallPortFilter "+
			"-AssociatedNetFirewallRule $rule\n"+
			"  if ($rule.Enabled -eq $true "+
			"      -and $rule.Direction -eq 'Inbound' "+
			"      -and $rule.Action -eq 'Block' "+
			"      -and $rule.Profile -eq 'Any' "+
			"      -and $filter.Protocol -eq 'TCP' "+
			"      -and $filter.LocalPort -eq '%d') {\n"+
			"    $matches = $true\n"+
			"    break\n"+
			"  }\n"+
			"}\n"+
			"[Console]::Out.Write($matches.ToString().ToLowerInvariant())\n",
		escapedRuleName,
		options.ListenPort,
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
			"query Libre Hardware Monitor firewall: %w",
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
			"unexpected Libre Hardware Monitor firewall query result %q",
			strings.TrimSpace(string(output)),
		)
	}
}

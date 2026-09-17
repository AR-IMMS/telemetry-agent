package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"
)

func TestFirewallMatchesReadsPowerShellTrueResult(t *testing.T) {
	matches, err := firewallMatches(
		context.Background(),
		DefaultOptions(),
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
				"Get-NetFirewallRule",
				"Get-NetFirewallPortFilter",
				"AR-IMMS Libre Hardware Monitor metrics",
				"9190",
			} {
				if !strings.Contains(script, want) {
					t.Fatalf(
						"firewall query script does not contain %q:\n%s",
						want,
						script,
					)
				}
			}

			return []byte("true\n"), nil
		},
	)
	if err != nil {
		t.Fatalf("firewallMatches() error = %v", err)
	}
	if !matches {
		t.Fatal("firewallMatches() = false, want true")
	}
}

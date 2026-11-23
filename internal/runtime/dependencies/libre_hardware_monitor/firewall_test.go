package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"
)

func TestConfigureFirewallReplacesManagedInboundBlockRule(t *testing.T) {
	var gotScript string

	err := ConfigureFirewall(
		context.Background(),
		DefaultOptions(),
		func(ctx context.Context, script string) error {
			gotScript = script
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ConfigureFirewall() error = %v", err)
	}

	for _, want := range []string{
		"Get-NetFirewallRule",
		"Remove-NetFirewallRule",
		"New-NetFirewallRule",
		"-DisplayName 'AR-IMMS Libre Hardware Monitor metrics'",
		"-Direction Inbound",
		"-Action Block",
		"-Protocol TCP",
		"-LocalPort 9190",
		"-Profile Any",
	} {
		if !strings.Contains(gotScript, want) {
			t.Fatalf(
				"firewall script does not contain %q:\n%s",
				want,
				gotScript,
			)
		}
	}
}

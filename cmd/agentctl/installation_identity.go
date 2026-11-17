package main

import (
	"crypto/rand"
	"fmt"
	installationapp "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"

	"strings"
)

// prepareAgentInstallationIdentity loads operational state before asking the
// Identity module to initialize or validate the durable installation UUID.
func prepareAgentInstallationIdentity(statePath string) error {
	state, err := newAgentStateStore(statePath).Load()
	if err != nil {
		return fmt.Errorf("load Agent state before identity initialization: %w", err)
	}
	collector := state.Collector
	if strings.TrimSpace(collector.BinaryPath) == "" ||
		strings.TrimSpace(collector.ConfigPath) == "" ||
		strings.TrimSpace(collector.GatewayEndpoint) == "" ||
		strings.TrimSpace(collector.HealthEndpoint) == "" {
		return fmt.Errorf("Agent operational state has incomplete Collector context")
	}
	if _, err := installationapp.EnsureInstallationIdentity(
		newAgentIdentityStore(statePath),
		rand.Reader,
	); err != nil {
		return fmt.Errorf("prepare Agent installation identity: %w", err)
	}
	return nil
}

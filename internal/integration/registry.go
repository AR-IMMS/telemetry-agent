package integration

import "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"

// Registry is a source-compatibility alias for the Monitoring-owned definition registry.
type Registry = definitions.Registry

// NewRegistry forwards shipped definition registration to Monitoring.
func NewRegistry(items ...IntegrationDefinition) (Registry, error) {
	return definitions.NewRegistry(items...)
}

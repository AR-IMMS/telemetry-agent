package integration

import definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"

// Registry is a source-compatibility alias for the Monitoring-owned definition registry.
type Registry = definitionapp.Registry

// NewRegistry forwards shipped definition registration to Monitoring.
func NewRegistry(items ...IntegrationDefinition) (Registry, error) {
	return definitionapp.NewRegistry(items...)
}

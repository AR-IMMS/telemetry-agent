package main

import (
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	redisdefinition "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/redis"
)

// productionIntegrationRegistry registers Monitoring-owned definitions independently from Runtime capability.
func productionIntegrationRegistry() (definitionapp.Registry, error) {
	return definitionapp.NewRegistry(redisdefinition.Definition())
}

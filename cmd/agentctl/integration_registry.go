package main

import (
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
	redisdefinition "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/redis"
)

// productionIntegrationRegistry registers Monitoring-owned definitions independently from Runtime capability.
func productionIntegrationRegistry() (definitions.Registry, error) {
	return definitions.NewRegistry(redisdefinition.Definition())
}

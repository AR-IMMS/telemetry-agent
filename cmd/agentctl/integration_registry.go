package main

import (
	"github.com/ar-imms/telemetry-agent/internal/integration"
	redisintegration "github.com/ar-imms/telemetry-agent/internal/integration/redis"
)

// productionIntegrationRegistry registers definitions independently from runtime capability.
func productionIntegrationRegistry() (integration.Registry, error) {
	return integration.NewRegistry(redisintegration.Definition())
}

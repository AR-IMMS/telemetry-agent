package main

import (
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	redisintegration "github.com/ar-imms/telemetry-agent/internal/integration/redis"
)

// productionIntegrationRegistry composes only definitions whose required
// capabilities are present in the verified platform catalog.
func productionIntegrationRegistry(catalog collectorruntime.Catalog) (integration.Registry, error) {
	definitions := make([]integration.IntegrationDefinition, 0, 1)
	for _, capability := range catalog.Manifest() {
		if capability.ID == "otel.receiver.redis.metrics" {
			definitions = append(definitions, redisintegration.Definition())
			break
		}
	}
	return integration.NewRegistry(definitions...)
}

package main

import (
	"context"
	"net/http"
	"runtime"
	"time"

	agentapp "github.com/ar-imms/telemetry-agent/internal/app"
	inventorycommand "github.com/ar-imms/telemetry-agent/internal/cli/commands/inventory"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	profilesapplication "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/application"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	collectorruntime "github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	agentinstallation "github.com/ar-imms/telemetry-agent/internal/runtime/service"
)

func inventoryCommandDependencies() inventorycommand.Dependencies {
	return inventorycommand.Dependencies{
		DefaultStatePath: agentinstallation.DefaultStatePath,
		LoadSnapshot: func(path string) (inventorydomain.InventorySnapshot, error) {
			return agentapp.NewInventoryRepository(path).Snapshot()
		},
		LoadProfiles: func(path string) (monitoringdomain.Registry, error) {
			registry := monitoringdomain.DefaultRegistry()
			state, err := newAgentStateStore(path).Load()
			if err != nil {
				return registry, nil
			}
			return loadMonitoringProfiles(state.Collector.ConfigRoot)
		},
		FetchCollectorScrapeEvidence: func(ctx context.Context) []profilesapplication.CollectorScrapeEvidence {
			return inventorycommand.FetchCollectorScrapeEvidence(ctx, &http.Client{Timeout: 400 * time.Millisecond}, defaultStatusEndpoint)
		},
		ProfileCredentialsConfigured: collectorruntime.ProfileCredentialsConfigured,
		DiagnoseEndpoint:             diagnoseInventoryEndpoint,
		Platform:                     runtime.GOOS,
	}
}

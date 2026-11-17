package main

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	"github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/installation"
	agentlifecycle "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation"
)

func newAgentSQLiteStore(path string) agentstate.SQLiteStore {
	return app.NewAgentSQLiteStore(path)
}

func newAgentStateStore(path string) agentstate.Repository {
	return app.NewAgentStateStore(path)
}

func newAgentIdentityStore(path string) installation.InstallationIdentityRepository {
	return app.NewInstallationIdentityStore(path)
}

func newAgentEnrollmentStore(path string) enrollment.AttemptRepository {
	return app.NewEnrollmentAttemptStore(path)
}

func newAgentCoreGatewayStore(path string) remoteconfig.GatewayRuntimeRepository {
	return app.NewCoreGatewayRuntimeStore(path)
}

func newAgentRuntimeStoreFactory() agentlifecycle.RuntimeStoreFactory {
	return func(path string) (agentstate.Repository, remoteconfig.GatewayRuntimeRepository, enrollment.AttemptRepository) {
		return newAgentStateStore(path), newAgentCoreGatewayStore(path), newAgentEnrollmentStore(path)
	}
}

func runAgentCollectorRuntime(ctx context.Context, path string, teardown agentlifecycle.DependencyTeardownFunc) error {
	return agentlifecycle.RunCollectorRuntime(ctx, path, teardown, newAgentRuntimeStoreFactory())
}

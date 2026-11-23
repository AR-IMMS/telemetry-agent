package main

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	"github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	installation "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
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

func newAgentFleetEnrollmentStore(path string) fleetenrollment.AttemptRepository {
	return app.NewFleetEnrollmentAttemptStore(path)
}
func newAgentCoreGatewayStore(path string) remoteconfig.GatewayRuntimeRepository {
	return app.NewCoreGatewayRuntimeStore(path)
}

func newAgentRuntimeStoreFactory() agentlifecycle.RuntimeStoreFactory {
	return func(path string) (agentstate.Repository, remoteconfig.GatewayRuntimeRepository, enrollment.AttemptRepository, fleetenrollment.AttemptRepository) {
		return newAgentStateStore(path), newAgentCoreGatewayStore(path), newAgentEnrollmentStore(path), newAgentFleetEnrollmentStore(path)
	}
}
func runAgentCollectorRuntime(ctx context.Context, path string, teardown agentlifecycle.DependencyTeardownFunc) error {
	return agentlifecycle.RunCollectorRuntime(ctx, path, teardown, newAgentRuntimeStoreFactory())
}

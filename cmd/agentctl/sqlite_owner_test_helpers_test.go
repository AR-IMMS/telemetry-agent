package main

import (
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	proposalapp "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func newTestDesiredStateRepository(store agentstate.SQLiteStore) desiredstateapp.Repository {
	return sqlitefixture.NewDesiredStateRepositoryFromStore(store)
}

func newTestTemplateRepository(store agentstate.SQLiteStore) templatesapp.Repository {
	return sqlitefixture.NewTemplateRepositoryFromStore(store)
}

func newTestMaterializationRepository(store agentstate.SQLiteStore) materializationapp.Repository {
	return sqlitefixture.NewMaterializationRepositoryFromStore(store)
}

func newTestApprovalRepository(store agentstate.SQLiteStore) proposalapp.ApprovalRepository {
	return sqlitefixture.NewApprovalRepositoryFromStore(store)
}

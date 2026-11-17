package main

import (
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func newTestDesiredStateRepository(store agentstate.SQLiteStore) desiredstateapp.Repository {
	return sqlitefixture.NewDesiredStateRepositoryFromStore(store)
}

func newTestTemplateRepository(store agentstate.SQLiteStore) templatesapp.Repository {
	return sqlitefixture.NewTemplateRepositoryFromStore(store)
}

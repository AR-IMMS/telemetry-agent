package main

import (
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func newTestDesiredStateRepository(store agentstate.SQLiteStore) desiredstateapp.Repository {
	return sqlitefixture.NewDesiredStateRepositoryFromStore(store)
}

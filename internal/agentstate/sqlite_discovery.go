package agentstate

import (
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
)

// ProposalRepository returns the Configuration-owned proposal adapter over this store.
func (s SQLiteStore) ProposalRepository() application.ProposalRepository {
	return configurationsqlite.NewProposalRepository(s.OpenDatabase, platformsqlite.UpdateLocker())
}

package agentstate

import (
	"github.com/ar-imms/telemetry-agent/internal/identity"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
)

// EnsureInstallationID preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) EnsureInstallationID(candidate identity.InstallationID) (identity.InstallationID, error) {
	return identitysqlite.New(s.OpenDatabase).EnsureInstallationID(candidate)
}

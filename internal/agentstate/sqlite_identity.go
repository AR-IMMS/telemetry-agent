package agentstate

import (
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
)

// EnsureInstallationID preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) EnsureInstallationID(candidate installationid.InstallationID) (installationid.InstallationID, error) {
	return identitysqlite.New(s.OpenDatabase).EnsureInstallationID(candidate)
}

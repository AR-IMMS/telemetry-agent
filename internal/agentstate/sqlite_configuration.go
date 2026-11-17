package agentstate

import (
	"database/sql"
)

func desiredIntegrationRevision(queryer interface {
	QueryRow(string, ...any) *sql.Row
}) (uint64, error) {
	var revision uint64
	err := queryer.QueryRow(`SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&revision)
	return revision, err
}

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
)

// MarkGatewayAppliedInTransaction records Collector readiness against the
// Configuration-owned Gateway snapshot in a caller-owned transaction. The
// caller controls the transaction boundary so Agent candidate attribution and
// Gateway last-known-good updates remain atomic.
func MarkGatewayAppliedInTransaction(tx *sql.Tx, generation uint64, endpoint string) error {
	if tx == nil {
		return fmt.Errorf("Gateway transaction is required")
	}
	if err := remoteconfig.ValidateGatewayEndpoint(endpoint); err != nil {
		return err
	}
	row, exists, err := ReadSnapshot(tx)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var lastGood string
	if err := tx.QueryRow(`SELECT last_good_endpoint FROM core_gateway_runtime WHERE id=1`).Scan(&lastGood); err != nil {
		return fmt.Errorf("load last-good Gateway endpoint: %w", err)
	}
	if row.Generation <= generation && row.Endpoint == endpoint {
		if _, err := tx.Exec(`UPDATE core_gateway_runtime SET last_good_endpoint=?,applied_revision=?,applied_generation=? WHERE id=1`, endpoint, row.Revision, generation); err != nil {
			return fmt.Errorf("persist applied Core Gateway revision: %w", err)
		}
		return nil
	}
	if lastGood == endpoint {
		if _, err := tx.Exec(`UPDATE core_gateway_runtime SET applied_generation=? WHERE id=1`, generation); err != nil {
			return fmt.Errorf("persist applied local Gateway generation: %w", err)
		}
		return nil
	}
	return fmt.Errorf("Gateway endpoint changed while generation %d was starting", generation)
}

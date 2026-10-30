package agentstate

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

type coreGatewayRow struct {
	revision   uint64
	endpoint   string
	proof      string
	generation uint64
}

// AcceptGatewaySnapshot atomically accepts one trusted, complete Core snapshot.
// It re-reads both the local state and prior snapshot under an immediate SQLite
// transaction, so local dependency changes cannot be lost by stale writes.
// Authentication is the caller's responsibility; S1 has no production ingress.
func (s SQLiteStore) AcceptGatewaySnapshot(snapshot coreconfig.GatewaySnapshot) (coreconfig.Acceptance, error) {
	if err := coreconfig.ValidateSnapshot(snapshot); err != nil {
		return coreconfig.Acceptance{}, err
	}
	current, err := s.Load()
	if err != nil {
		return coreconfig.Acceptance{}, fmt.Errorf("load Agent state before Core Gateway acceptance: %w", err)
	}
	if configPath := current.Collector.ConfigPath; configPath != "" {
		lifecycleLock, err := AcquireLifecycleLock(configPath + ".lifecycle")
		if err != nil {
			return coreconfig.Acceptance{}, fmt.Errorf("acquire lifecycle lock for Core Gateway acceptance: %w", err)
		}
		defer lifecycleLock.Close()
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
	if err != nil {
		return coreconfig.Acceptance{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return coreconfig.Acceptance{}, fmt.Errorf("begin Core Gateway acceptance: %w", err)
	}
	defer tx.Rollback()
	if err := checkGatewayInstallation(tx, snapshot.InstallationID); err != nil {
		return coreconfig.Acceptance{}, err
	}
	state, err := loadSQLiteState(tx)
	if err != nil {
		return coreconfig.Acceptance{}, fmt.Errorf("accept Core Gateway without committed Agent state: %w", err)
	}
	prior, exists, err := loadCoreGatewayRow(tx)
	if err != nil {
		return coreconfig.Acceptance{}, err
	}
	if exists {
		switch {
		case snapshot.Revision < prior.revision:
			return coreconfig.Acceptance{}, fmt.Errorf("Core Gateway revision %d is below accepted revision %d: %w", snapshot.Revision, prior.revision, coreconfig.ErrStaleRevision)
		case snapshot.Revision == prior.revision:
			if gatewayFingerprint(snapshot.Endpoint) != prior.proof {
				return coreconfig.Acceptance{}, fmt.Errorf("Core Gateway revision %d has different content: %w", snapshot.Revision, coreconfig.ErrRevisionConflict)
			}
			return coreconfig.Acceptance{Revision: prior.revision, Generation: prior.generation}, nil
		}
	}
	previousEndpoint := state.Collector.GatewayEndpoint
	if exists {
		previousEndpoint = prior.endpoint
	}
	changed := snapshot.Endpoint != previousEndpoint
	if changed {
		if state.DesiredGeneration >= math.MaxInt64 {
			return coreconfig.Acceptance{}, fmt.Errorf("local desired generation cannot advance beyond SQLite integer range")
		}
		state.DesiredGeneration++
		if err := persistGatewayGeneration(tx, state); err != nil {
			return coreconfig.Acceptance{}, err
		}
	}
	// For an unchanged endpoint, bind the new Core revision to the current
	// desired generation, which may include unrelated local dependency edits.
	if state.DesiredGeneration > math.MaxInt64 {
		return coreconfig.Acceptance{}, fmt.Errorf("local desired generation exceeds SQLite integer range")
	}
	if _, err := tx.Exec(`INSERT INTO core_gateway_snapshot
		(id, revision, endpoint, fingerprint, accepted_at, target_generation)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,
		endpoint=excluded.endpoint, fingerprint=excluded.fingerprint,
		accepted_at=excluded.accepted_at, target_generation=excluded.target_generation`,
		snapshot.Revision, snapshot.Endpoint, gatewayFingerprint(snapshot.Endpoint),
		time.Now().UTC().Format(time.RFC3339Nano), state.DesiredGeneration,
	); err != nil {
		return coreconfig.Acceptance{}, fmt.Errorf("persist Core Gateway snapshot: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO core_gateway_runtime
		(id, last_good_endpoint, activated_revision, activated_generation, applied_revision, applied_generation)
		VALUES (1, ?, 0, ?, 0, ?)
		ON CONFLICT(id) DO NOTHING`, previousEndpoint, state.ActivatedGeneration, state.AppliedGeneration); err != nil {
		return coreconfig.Acceptance{}, fmt.Errorf("initialize Core Gateway runtime state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return coreconfig.Acceptance{}, fmt.Errorf("commit Core Gateway snapshot and local generation: %w", err)
	}
	return coreconfig.Acceptance{Revision: snapshot.Revision, Generation: state.DesiredGeneration, Changed: changed}, nil
}

// GatewayForGeneration resolves a launch endpoint from the candidate snapshot
// only after its local generation is activated; otherwise it returns last-good.
func (s SQLiteStore) GatewayForGeneration(generation uint64) (coreconfig.RuntimeGateway, error) {
	db, err := s.open()
	if err != nil {
		return coreconfig.RuntimeGateway{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return coreconfig.RuntimeGateway{}, fmt.Errorf("begin Gateway runtime resolution: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSQLiteState(tx)
	if err != nil {
		return coreconfig.RuntimeGateway{}, err
	}
	row, exists, err := loadCoreGatewayRow(tx)
	if err != nil {
		return coreconfig.RuntimeGateway{}, err
	}
	runtime := coreconfig.RuntimeGateway{Endpoint: state.Collector.GatewayEndpoint, Generation: state.AppliedGeneration}
	if exists {
		var lastGood string
		var appliedRevision, appliedGeneration int64
		if err := tx.QueryRow(`SELECT last_good_endpoint, applied_revision, applied_generation FROM core_gateway_runtime WHERE id = 1`).
			Scan(&lastGood, &appliedRevision, &appliedGeneration); err != nil {
			return coreconfig.RuntimeGateway{}, fmt.Errorf("load Core Gateway last-good runtime: %w", err)
		}
		if row.generation <= generation {
			runtime = coreconfig.RuntimeGateway{
				Endpoint: row.endpoint, Revision: row.revision, Generation: row.generation,
				AppliedRevision: uint64(appliedRevision),
				NeedsReadiness:  row.endpoint == lastGood && uint64(appliedRevision) < row.revision,
			}
		} else {
			runtime = coreconfig.RuntimeGateway{
				Endpoint: lastGood, Revision: uint64(appliedRevision), Generation: uint64(appliedGeneration),
				AppliedRevision: uint64(appliedRevision),
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return coreconfig.RuntimeGateway{}, fmt.Errorf("finish Gateway runtime resolution: %w", err)
	}
	return runtime, nil
}

// MarkGatewayActivated records which accepted Core revision passed Collector
// configuration validation for the supplied local generation.
func (s SQLiteStore) MarkGatewayActivated(generation uint64) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Core Gateway activation mapping: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSQLiteState(tx)
	if err != nil {
		return err
	}
	if state.ActivatedGeneration < generation {
		return fmt.Errorf("Core Gateway generation %d is not activated", generation)
	}
	row, exists, err := loadCoreGatewayRow(tx)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if row.generation > generation {
		// A newer accepted snapshot stays pending; it must not make completion
		// of this older, already-activated local generation look like a failure.
		return nil
	}
	if _, err := tx.Exec(`UPDATE core_gateway_runtime SET activated_revision = ?, activated_generation = ? WHERE id = 1`, row.revision, generation); err != nil {
		return fmt.Errorf("persist activated Core Gateway revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Core Gateway activation mapping: %w", err)
	}
	return nil
}

// MarkGatewayApplied commits local applied state and its endpoint atomically.
func (s SQLiteStore) MarkGatewayApplied(generation uint64, endpoint string) error {
	if err := coreconfig.ValidateGatewayEndpoint(endpoint); err != nil {
		return err
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Gateway applied-state transaction: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSQLiteState(tx)
	if err != nil {
		return err
	}
	if err := state.MarkGenerationApplied(generation); err != nil {
		return err
	}
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Gateway applied state: %w", err)
	}
	if _, err := tx.Exec(`UPDATE agent_state SET payload = ?, updated_at = ? WHERE id = 1`, content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist Gateway applied generation: %w", err)
	}
	row, exists, err := loadCoreGatewayRow(tx)
	if err != nil {
		return err
	}
	if exists {
		var lastGood string
		if err := tx.QueryRow(`SELECT last_good_endpoint FROM core_gateway_runtime WHERE id = 1`).Scan(&lastGood); err != nil {
			return fmt.Errorf("load last-good Gateway endpoint: %w", err)
		}
		if row.generation <= generation && row.endpoint == endpoint {
			lastGood = endpoint
			if _, err := tx.Exec(`UPDATE core_gateway_runtime SET last_good_endpoint = ?, applied_revision = ?, applied_generation = ? WHERE id = 1`, lastGood, row.revision, generation); err != nil {
				return fmt.Errorf("persist applied Core Gateway revision: %w", err)
			}
		} else if lastGood == endpoint {
			if _, err := tx.Exec(`UPDATE core_gateway_runtime SET applied_generation = ? WHERE id = 1`, generation); err != nil {
				return fmt.Errorf("persist applied local Gateway generation: %w", err)
			}
		} else {
			return fmt.Errorf("Gateway endpoint changed while generation %d was starting", generation)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Gateway applied endpoint and generation: %w", err)
	}
	return nil
}

// EffectiveGateway resolves the committed Core endpoint, or the Bootstrap
// seed when no Core ownership row exists. State and snapshot share one read
// transaction so the reported generation and endpoint belong together.
func (s SQLiteStore) EffectiveGateway() (coreconfig.EffectiveGateway, error) {
	db, err := s.open()
	if err != nil {
		return coreconfig.EffectiveGateway{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return coreconfig.EffectiveGateway{}, fmt.Errorf("begin effective Gateway read: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSQLiteState(tx)
	if err != nil {
		return coreconfig.EffectiveGateway{}, err
	}
	row, exists, err := loadCoreGatewayRow(tx)
	if err != nil {
		return coreconfig.EffectiveGateway{}, err
	}
	effective := coreconfig.EffectiveGateway{
		Owner: coreconfig.BootstrapOwner, Endpoint: state.Collector.GatewayEndpoint,
		Generation: state.DesiredGeneration,
	}
	if exists {
		effective = coreconfig.EffectiveGateway{
			Owner: coreconfig.CoreOwner, Endpoint: row.endpoint,
			Revision: row.revision, Generation: row.generation,
		}
	}
	if err := tx.Commit(); err != nil {
		return coreconfig.EffectiveGateway{}, fmt.Errorf("finish effective Gateway read: %w", err)
	}
	return effective, nil
}

func checkGatewayInstallation(tx *sql.Tx, supplied string) error {
	initialized, stored, err := loadInstallationIdentity(tx)
	if err != nil {
		return err
	}
	if !initialized || !stored.Valid {
		return fmt.Errorf("Core Gateway snapshot requires an initialized installation identity")
	}
	committed, err := identity.ParseInstallationID(stored.String)
	if err != nil {
		return fmt.Errorf("validate committed installation identity: %w", err)
	}
	incoming, err := identity.ParseInstallationID(supplied)
	if err != nil {
		return fmt.Errorf("validate snapshot installation identity: %w", err)
	}
	if committed != incoming {
		return fmt.Errorf("Core Gateway snapshot installation identity does not match local installation")
	}
	return nil
}

func loadCoreGatewayRow(tx *sql.Tx) (coreGatewayRow, bool, error) {
	var row coreGatewayRow
	var revision, generation int64
	err := tx.QueryRow(`SELECT revision, endpoint, fingerprint, target_generation FROM core_gateway_snapshot WHERE id = 1`).
		Scan(&revision, &row.endpoint, &row.proof, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return coreGatewayRow{}, false, nil
	}
	if err != nil {
		return coreGatewayRow{}, false, fmt.Errorf("load Core Gateway snapshot: %w", err)
	}
	if revision <= 0 || generation < 0 || coreconfig.ValidateGatewayEndpoint(row.endpoint) != nil || row.proof != gatewayFingerprint(row.endpoint) {
		return coreGatewayRow{}, false, fmt.Errorf("committed Core Gateway snapshot is invalid")
	}
	row.revision, row.generation = uint64(revision), uint64(generation)
	return row, true, nil
}

func persistGatewayGeneration(tx *sql.Tx, state State) error {
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Agent state for Core Gateway: %w", err)
	}
	result, err := tx.Exec(`UPDATE agent_state SET payload = ?, updated_at = ? WHERE id = 1`,
		content, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("persist Agent generation for Core Gateway: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Agent generation writes for Core Gateway: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("persist Agent generation for Core Gateway: expected one state row, got %d", count)
	}
	return nil
}

func gatewayFingerprint(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

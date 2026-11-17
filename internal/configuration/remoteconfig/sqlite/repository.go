package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

// AgentState is the subset of Agent-owned lifecycle state needed to preserve
// Gateway generation and fallback semantics inside the shared transaction.
type AgentState struct {
	BootstrapEndpoint   string
	DesiredGeneration   uint64
	ActivatedGeneration uint64
	AppliedGeneration   uint64
}

// StateAccess supplies Agent/Runtime fields that share atomic transactions with
// Configuration snapshots. Implementations do not own Gateway tables.
type StateAccess struct {
	BeginSnapshotAcceptance func() (func(), error)
	ReadInstallationID      func(*sql.Tx) (string, bool, error)
	Read                    func(*sql.Tx) (AgentState, error)
	SetDesiredGeneration    func(*sql.Tx, uint64) error
	MarkGenerationApplied   func(*sql.Tx, uint64) error
}

// Repository persists accepted Core snapshots and their Runtime attribution.
type Repository struct {
	open  func(context.Context) (*sql.DB, error)
	lock  sync.Locker
	state StateAccess
}

// New creates a Gateway SQLite repository over the supplied shared database.
func New(open func(context.Context) (*sql.DB, error), lock sync.Locker, state StateAccess) *Repository {
	return &Repository{open: open, lock: lock, state: state}
}

type gatewayRow struct {
	revision   uint64
	endpoint   string
	proof      string
	generation uint64
}

// AcceptGatewaySnapshot atomically accepts a trusted Core snapshot and advances
// local desired generation when its endpoint changes.
func (s *Repository) AcceptGatewaySnapshot(ctx context.Context, snapshot remoteconfig.GatewaySnapshot) (remoteconfig.Acceptance, error) {
	if err := validateContext(ctx); err != nil {
		return remoteconfig.Acceptance{}, err
	}
	if err := remoteconfig.ValidateSnapshot(snapshot); err != nil {
		return remoteconfig.Acceptance{}, err
	}
	if s.state.BeginSnapshotAcceptance != nil {
		release, err := s.state.BeginSnapshotAcceptance()
		if err != nil {
			return remoteconfig.Acceptance{}, err
		}
		if release != nil {
			defer release()
		}
	}
	db, err := s.openDatabase(ctx)
	if err != nil {
		return remoteconfig.Acceptance{}, err
	}
	defer db.Close()
	if s.lock != nil {
		s.lock.Lock()
		defer s.lock.Unlock()
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return remoteconfig.Acceptance{}, fmt.Errorf("begin Core Gateway acceptance: %w", err)
	}
	defer tx.Rollback()
	if err := s.checkInstallation(tx, snapshot.InstallationID); err != nil {
		return remoteconfig.Acceptance{}, err
	}
	state, err := s.readState(tx)
	if err != nil {
		return remoteconfig.Acceptance{}, fmt.Errorf("load Agent state before Core Gateway acceptance: %w", err)
	}
	prior, exists, err := readSnapshot(ctx, tx)
	if err != nil {
		return remoteconfig.Acceptance{}, err
	}
	if exists {
		switch {
		case snapshot.Revision < prior.revision:
			return remoteconfig.Acceptance{}, fmt.Errorf("Core Gateway revision %d is below accepted revision %d: %w", snapshot.Revision, prior.revision, remoteconfig.ErrStaleRevision)
		case snapshot.Revision == prior.revision:
			if fingerprint(snapshot.Endpoint) != prior.proof {
				return remoteconfig.Acceptance{}, fmt.Errorf("Core Gateway revision %d has different content: %w", snapshot.Revision, remoteconfig.ErrRevisionConflict)
			}
			return remoteconfig.Acceptance{Revision: prior.revision, Generation: prior.generation}, nil
		}
	}
	previousEndpoint := state.BootstrapEndpoint
	if exists {
		previousEndpoint = prior.endpoint
	}
	changed := snapshot.Endpoint != previousEndpoint
	if changed {
		if state.DesiredGeneration >= math.MaxInt64 {
			return remoteconfig.Acceptance{}, errors.New("local desired generation cannot advance beyond SQLite integer range")
		}
		state.DesiredGeneration++
		if s.state.SetDesiredGeneration == nil {
			return remoteconfig.Acceptance{}, errors.New("Core Gateway desired-generation writer is not configured")
		}
		if err := s.state.SetDesiredGeneration(tx, state.DesiredGeneration); err != nil {
			return remoteconfig.Acceptance{}, err
		}
	}
	if state.DesiredGeneration > math.MaxInt64 {
		return remoteconfig.Acceptance{}, errors.New("local desired generation exceeds SQLite integer range")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO core_gateway_snapshot
		(id, revision, endpoint, fingerprint, accepted_at, target_generation)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,
		endpoint=excluded.endpoint, fingerprint=excluded.fingerprint,
		accepted_at=excluded.accepted_at, target_generation=excluded.target_generation`,
		snapshot.Revision, snapshot.Endpoint, fingerprint(snapshot.Endpoint), time.Now().UTC().Format(time.RFC3339Nano), state.DesiredGeneration); err != nil {
		return remoteconfig.Acceptance{}, fmt.Errorf("persist Core Gateway snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO core_gateway_runtime
		(id,last_good_endpoint,activated_revision,activated_generation,applied_revision,applied_generation)
		VALUES (1, ?, 0, ?, 0, ?) ON CONFLICT(id) DO NOTHING`, previousEndpoint, state.ActivatedGeneration, state.AppliedGeneration); err != nil {
		return remoteconfig.Acceptance{}, fmt.Errorf("initialize Core Gateway runtime state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return remoteconfig.Acceptance{}, fmt.Errorf("commit Core Gateway snapshot and local generation: %w", err)
	}
	return remoteconfig.Acceptance{Revision: snapshot.Revision, Generation: state.DesiredGeneration, Changed: changed}, nil
}

// EffectiveGateway resolves Core authority or the Bootstrap endpoint in one
// transaction so its generation and endpoint are observed together.
func (s *Repository) EffectiveGateway(ctx context.Context) (remoteconfig.EffectiveGateway, error) {
	if err := validateContext(ctx); err != nil {
		return remoteconfig.EffectiveGateway{}, err
	}
	db, err := s.openDatabase(ctx)
	if err != nil {
		return remoteconfig.EffectiveGateway{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return remoteconfig.EffectiveGateway{}, fmt.Errorf("begin effective Gateway read: %w", err)
	}
	defer tx.Rollback()
	state, err := s.readState(tx)
	if err != nil {
		return remoteconfig.EffectiveGateway{}, err
	}
	row, exists, err := readSnapshot(ctx, tx)
	if err != nil {
		return remoteconfig.EffectiveGateway{}, err
	}
	effective := remoteconfig.EffectiveGateway{Owner: remoteconfig.BootstrapOwner, Endpoint: state.BootstrapEndpoint, Generation: state.DesiredGeneration}
	if exists {
		effective = remoteconfig.EffectiveGateway{Owner: remoteconfig.CoreOwner, Endpoint: row.endpoint, Revision: row.revision, Generation: row.generation}
	}
	if err := tx.Commit(); err != nil {
		return remoteconfig.EffectiveGateway{}, fmt.Errorf("finish effective Gateway read: %w", err)
	}
	return effective, nil
}

// GatewayForGeneration resolves the candidate endpoint only after activation;
// otherwise it retains the last known-good endpoint.
func (s *Repository) GatewayForGeneration(ctx context.Context, generation uint64) (remoteconfig.RuntimeGateway, error) {
	if err := validateContext(ctx); err != nil {
		return remoteconfig.RuntimeGateway{}, err
	}
	db, err := s.openDatabase(ctx)
	if err != nil {
		return remoteconfig.RuntimeGateway{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return remoteconfig.RuntimeGateway{}, fmt.Errorf("begin Gateway runtime resolution: %w", err)
	}
	defer tx.Rollback()
	state, err := s.readState(tx)
	if err != nil {
		return remoteconfig.RuntimeGateway{}, err
	}
	row, exists, err := readSnapshot(ctx, tx)
	if err != nil {
		return remoteconfig.RuntimeGateway{}, err
	}
	runtime := remoteconfig.RuntimeGateway{Endpoint: state.BootstrapEndpoint, Generation: state.AppliedGeneration}
	if exists {
		var lastGood string
		var appliedRevision, appliedGeneration int64
		if err := tx.QueryRowContext(ctx, `SELECT last_good_endpoint,applied_revision,applied_generation FROM core_gateway_runtime WHERE id=1`).Scan(&lastGood, &appliedRevision, &appliedGeneration); err != nil {
			return remoteconfig.RuntimeGateway{}, fmt.Errorf("load Core Gateway last-good runtime: %w", err)
		}
		if row.generation <= generation {
			runtime = remoteconfig.RuntimeGateway{Endpoint: row.endpoint, Revision: row.revision, Generation: row.generation, AppliedRevision: uint64(appliedRevision), NeedsReadiness: row.endpoint == lastGood && uint64(appliedRevision) < row.revision}
		} else {
			runtime = remoteconfig.RuntimeGateway{Endpoint: lastGood, Revision: uint64(appliedRevision), Generation: uint64(appliedGeneration), AppliedRevision: uint64(appliedRevision)}
		}
	}
	if err := tx.Commit(); err != nil {
		return remoteconfig.RuntimeGateway{}, fmt.Errorf("finish Gateway runtime resolution: %w", err)
	}
	return runtime, nil
}

// MarkGatewayActivated associates a validated local generation with the
// accepted Core revision unless a newer revision is already pending.
func (s *Repository) MarkGatewayActivated(ctx context.Context, generation uint64) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	db, err := s.openDatabase(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Core Gateway activation mapping: %w", err)
	}
	defer tx.Rollback()
	state, err := s.readState(tx)
	if err != nil {
		return err
	}
	if state.ActivatedGeneration < generation {
		return fmt.Errorf("Core Gateway generation %d is not activated", generation)
	}
	row, exists, err := readSnapshot(ctx, tx)
	if err != nil {
		return err
	}
	if !exists || row.generation > generation {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE core_gateway_runtime SET activated_revision=?,activated_generation=? WHERE id=1`, row.revision, generation); err != nil {
		return fmt.Errorf("persist activated Core Gateway revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Core Gateway activation mapping: %w", err)
	}
	return nil
}

// MarkGatewayApplied commits the local applied generation and its Gateway
// endpoint attribution in one transaction.
func (s *Repository) MarkGatewayApplied(ctx context.Context, generation uint64, endpoint string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	if err := remoteconfig.ValidateGatewayEndpoint(endpoint); err != nil {
		return err
	}
	db, err := s.openDatabase(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	if s.lock != nil {
		s.lock.Lock()
		defer s.lock.Unlock()
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Gateway applied-state transaction: %w", err)
	}
	defer tx.Rollback()
	if s.state.MarkGenerationApplied == nil {
		return errors.New("Gateway applied-state callback is not configured")
	}
	if err := s.state.MarkGenerationApplied(tx, generation); err != nil {
		return err
	}
	row, exists, err := readSnapshot(ctx, tx)
	if err != nil {
		return err
	}
	if exists {
		var lastGood string
		if err := tx.QueryRowContext(ctx, `SELECT last_good_endpoint FROM core_gateway_runtime WHERE id=1`).Scan(&lastGood); err != nil {
			return fmt.Errorf("load last-good Gateway endpoint: %w", err)
		}
		if row.generation <= generation && row.endpoint == endpoint {
			if _, err := tx.ExecContext(ctx, `UPDATE core_gateway_runtime SET last_good_endpoint=?,applied_revision=?,applied_generation=? WHERE id=1`, endpoint, row.revision, generation); err != nil {
				return fmt.Errorf("persist applied Core Gateway revision: %w", err)
			}
		} else if lastGood == endpoint {
			if _, err := tx.ExecContext(ctx, `UPDATE core_gateway_runtime SET applied_generation=? WHERE id=1`, generation); err != nil {
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

// SnapshotRow is the accepted snapshot evidence used by Runtime transactions.
type SnapshotRow struct {
	Revision    uint64
	Endpoint    string
	Fingerprint string
	Generation  uint64
}

// ReadSnapshot reads Configuration-owned snapshot evidence in a caller's transaction.
func ReadSnapshot(tx *sql.Tx) (SnapshotRow, bool, error) {
	return ReadSnapshotContext(context.Background(), tx)
}

// ReadSnapshotContext reads Gateway evidence within the caller's transaction context.
func ReadSnapshotContext(ctx context.Context, tx *sql.Tx) (SnapshotRow, bool, error) {
	if err := validateContext(ctx); err != nil {
		return SnapshotRow{}, false, err
	}
	row := SnapshotRow{}
	var revision, generation int64
	err := tx.QueryRowContext(ctx, `SELECT revision,endpoint,fingerprint,target_generation FROM core_gateway_snapshot WHERE id=1`).Scan(&revision, &row.Endpoint, &row.Fingerprint, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return SnapshotRow{}, false, nil
	}
	if err != nil {
		return SnapshotRow{}, false, fmt.Errorf("load Core Gateway snapshot: %w", err)
	}
	if revision <= 0 || generation < 0 || remoteconfig.ValidateGatewayEndpoint(row.Endpoint) != nil || row.Fingerprint != fingerprint(row.Endpoint) {
		return SnapshotRow{}, false, errors.New("committed Core Gateway snapshot is invalid")
	}
	row.Revision, row.Generation = uint64(revision), uint64(generation)
	return row, true, nil
}

func readSnapshot(ctx context.Context, tx *sql.Tx) (gatewayRow, bool, error) {
	row, exists, err := ReadSnapshotContext(ctx, tx)
	if err != nil || !exists {
		return gatewayRow{}, exists, err
	}
	return gatewayRow{revision: row.Revision, endpoint: row.Endpoint, proof: row.Fingerprint, generation: row.Generation}, true, nil
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Core Gateway context is required")
	}
	return ctx.Err()
}

func (s *Repository) openDatabase(ctx context.Context) (*sql.DB, error) {
	if ctx == nil {
		return nil, errors.New("Core Gateway context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.open == nil {
		return nil, errors.New("Core Gateway database opener is not configured")
	}
	return s.open(ctx)
}
func (s *Repository) readState(tx *sql.Tx) (AgentState, error) {
	if s == nil || s.state.Read == nil {
		return AgentState{}, errors.New("Core Gateway state reader is not configured")
	}
	return s.state.Read(tx)
}

func (s *Repository) checkInstallation(tx *sql.Tx, supplied string) error {
	if s == nil || s.state.ReadInstallationID == nil {
		return errors.New("Core Gateway installation identity reader is not configured")
	}
	committed, initialized, err := s.state.ReadInstallationID(tx)
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("Core Gateway snapshot requires an initialized installation identity")
	}
	incoming, err := installationid.ParseInstallationID(supplied)
	if err != nil {
		return fmt.Errorf("validate snapshot installation identity: %w", err)
	}
	if committed != incoming.String() {
		return errors.New("Core Gateway snapshot installation identity does not match local installation")
	}
	return nil
}

func fingerprint(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

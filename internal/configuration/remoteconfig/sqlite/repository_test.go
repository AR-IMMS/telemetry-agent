package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	_ "modernc.org/sqlite"
)

const testInstallationID = "11111111-2222-4333-8444-555555555555"

func TestRepositoryOwnsSnapshotRevisionAndRuntimeAttribution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db := openGatewayTestDB(t, path)
	for _, statement := range []string{
		`CREATE TABLE test_agent_state (id INTEGER PRIMARY KEY, bootstrap_endpoint TEXT NOT NULL, desired_generation INTEGER NOT NULL, activated_generation INTEGER NOT NULL, applied_generation INTEGER NOT NULL)`,
		`INSERT INTO test_agent_state VALUES (1, 'bootstrap:4317', 1, 1, 1)`,
		`CREATE TABLE core_gateway_snapshot (id INTEGER PRIMARY KEY, revision INTEGER NOT NULL, endpoint TEXT NOT NULL, fingerprint TEXT NOT NULL, accepted_at TEXT NOT NULL, target_generation INTEGER NOT NULL)`,
		`CREATE TABLE core_gateway_runtime (id INTEGER PRIMARY KEY, last_good_endpoint TEXT NOT NULL, activated_revision INTEGER NOT NULL, activated_generation INTEGER NOT NULL, applied_revision INTEGER NOT NULL, applied_generation INTEGER NOT NULL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repository := newGatewayTestRepository(path)
	snapshot := remoteconfig.GatewaySnapshot{InstallationID: testInstallationID, Revision: 7, Endpoint: "gateway:4317"}
	accepted, err := repository.AcceptGatewaySnapshot(snapshot)
	if err != nil {
		t.Fatalf("AcceptGatewaySnapshot() error = %v", err)
	}
	if accepted.Generation != 2 || !accepted.Changed {
		t.Fatalf("AcceptGatewaySnapshot() = %+v, want changed generation 2", accepted)
	}
	if duplicate, err := repository.AcceptGatewaySnapshot(snapshot); err != nil || duplicate.Changed || duplicate.Generation != accepted.Generation {
		t.Fatalf("idempotent AcceptGatewaySnapshot() = %+v, %v", duplicate, err)
	}
	if _, err := repository.AcceptGatewaySnapshot(remoteconfig.GatewaySnapshot{InstallationID: testInstallationID, Revision: 6, Endpoint: "other:4317"}); !errors.Is(err, remoteconfig.ErrStaleRevision) {
		t.Fatalf("stale revision error = %v", err)
	}
	if _, err := repository.AcceptGatewaySnapshot(remoteconfig.GatewaySnapshot{InstallationID: testInstallationID, Revision: 7, Endpoint: "other:4317"}); !errors.Is(err, remoteconfig.ErrRevisionConflict) {
		t.Fatalf("revision conflict error = %v", err)
	}

	effective, err := repository.EffectiveGateway()
	if err != nil || effective.Owner != remoteconfig.CoreOwner || effective.Endpoint != "gateway:4317" || effective.Generation != 2 {
		t.Fatalf("EffectiveGateway() = %+v, %v", effective, err)
	}
	setGatewayTestState(t, path, "UPDATE test_agent_state SET activated_generation=2 WHERE id=1")
	if err := repository.MarkGatewayActivated(2); err != nil {
		t.Fatalf("MarkGatewayActivated() error = %v", err)
	}
	if err := repository.MarkGatewayApplied(2, "gateway:4317"); err != nil {
		t.Fatalf("MarkGatewayApplied() error = %v", err)
	}
	runtime, err := repository.GatewayForGeneration(2)
	if err != nil || runtime.Endpoint != "gateway:4317" || runtime.Revision != 7 || runtime.AppliedRevision != 7 {
		t.Fatalf("GatewayForGeneration() = %+v, %v", runtime, err)
	}

	if _, err := repository.AcceptGatewaySnapshot(remoteconfig.GatewaySnapshot{InstallationID: testInstallationID, Revision: 8, Endpoint: "next:4317"}); err != nil {
		t.Fatalf("AcceptGatewaySnapshot(next) error = %v", err)
	}
	lastGood, err := repository.GatewayForGeneration(2)
	if err != nil || lastGood.Endpoint != "gateway:4317" || lastGood.AppliedRevision != 7 {
		t.Fatalf("GatewayForGeneration() before next activation = %+v, %v", lastGood, err)
	}
}

func newGatewayTestRepository(path string) *Repository {
	return New(func() (*sql.DB, error) { return sql.Open("sqlite", path) }, nil, StateAccess{
		ReadInstallationID: func(*sql.Tx) (string, bool, error) { return testInstallationID, true, nil },
		Read: func(tx *sql.Tx) (AgentState, error) {
			var state AgentState
			err := tx.QueryRow(`SELECT bootstrap_endpoint,desired_generation,activated_generation,applied_generation FROM test_agent_state WHERE id=1`).Scan(&state.BootstrapEndpoint, &state.DesiredGeneration, &state.ActivatedGeneration, &state.AppliedGeneration)
			return state, err
		},
		SetDesiredGeneration: func(tx *sql.Tx, generation uint64) error {
			_, err := tx.Exec(`UPDATE test_agent_state SET desired_generation=? WHERE id=1`, generation)
			return err
		},
		MarkGenerationApplied: func(tx *sql.Tx, generation uint64) error {
			_, err := tx.Exec(`UPDATE test_agent_state SET applied_generation=? WHERE id=1`, generation)
			return err
		},
	})
}

func openGatewayTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func setGatewayTestState(t *testing.T, path, statement string) {
	t.Helper()
	db := openGatewayTestDB(t, path)
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

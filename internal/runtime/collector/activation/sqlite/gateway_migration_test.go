package sqlite

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
	_ "modernc.org/sqlite"
)

func TestCoreGatewayRuntimeMigrationSeedsRuntimeEvidenceFromAcceptedSnapshot(t *testing.T) {
	db := openGatewayMigrationTestDB(t)
	if err := migrate.InitializeLedger(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE core_gateway_snapshot (
		id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL,
		endpoint TEXT NOT NULL, fingerprint TEXT NOT NULL, accepted_at TEXT NOT NULL,
		target_generation INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO core_gateway_snapshot
		(id,revision,endpoint,fingerprint,accepted_at,target_generation)
		VALUES(1,7,'gateway.example:4317','proof','now',4)`); err != nil {
		t.Fatal(err)
	}

	migration := CoreGatewayRuntimeMigration(
		func(*sql.Tx) (GatewayMigrationState, error) {
			return GatewayMigrationState{
				BootstrapEndpoint:   "127.0.0.1:4317",
				ActivatedGeneration: 4,
				AppliedGeneration:   4,
			}, nil
		},
		func(tx *sql.Tx) (AcceptedGatewaySnapshot, bool, error) {
			var snapshot AcceptedGatewaySnapshot
			err := tx.QueryRow(`SELECT revision,endpoint,target_generation
				FROM core_gateway_snapshot WHERE id=1`).Scan(
				&snapshot.Revision, &snapshot.Endpoint, &snapshot.Generation,
			)
			if errors.Is(err, sql.ErrNoRows) {
				return AcceptedGatewaySnapshot{}, false, nil
			}
			return snapshot, err == nil, err
		},
	)
	if err := migration.Apply(db); err != nil {
		t.Fatalf("apply Runtime migration: %v", err)
	}

	var endpoint string
	var activatedRevision, activatedGeneration, appliedRevision, appliedGeneration uint64
	if err := db.QueryRow(`SELECT last_good_endpoint,activated_revision,activated_generation,
		applied_revision,applied_generation FROM core_gateway_runtime WHERE id=1`).Scan(
		&endpoint, &activatedRevision, &activatedGeneration, &appliedRevision, &appliedGeneration,
	); err != nil {
		t.Fatal(err)
	}
	if endpoint != "gateway.example:4317" || activatedRevision != 7 || activatedGeneration != 4 || appliedRevision != 7 || appliedGeneration != 4 {
		t.Fatalf("Runtime Gateway migration state=(%q,%d,%d,%d,%d), want accepted endpoint/revision and existing generations", endpoint, activatedRevision, activatedGeneration, appliedRevision, appliedGeneration)
	}
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=4`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("version 4 records=%d, want 1", applied)
	}
}

func TestCoreGatewayRuntimeMigrationRollsBackSchemaWhenLegacyStateReadFails(t *testing.T) {
	db := openGatewayMigrationTestDB(t)
	if err := migrate.InitializeLedger(db); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("legacy Runtime state unavailable")
	migration := CoreGatewayRuntimeMigration(
		func(*sql.Tx) (GatewayMigrationState, error) {
			return GatewayMigrationState{}, wantErr
		},
		func(*sql.Tx) (AcceptedGatewaySnapshot, bool, error) {
			t.Fatal("snapshot reader called after Runtime state read failed")
			return AcceptedGatewaySnapshot{}, false, nil
		},
	)
	if err := migration.Apply(db); !errors.Is(err, wantErr) {
		t.Fatalf("apply Runtime migration error=%v, want %v", err, wantErr)
	}
	for _, table := range []string{"core_gateway_runtime"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("table %q exists after failed migration, want rollback", table)
		}
	}
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=4`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("version 4 records=%d after failed migration, want 0", applied)
	}
}

func openGatewayMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

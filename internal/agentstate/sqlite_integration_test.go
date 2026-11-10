package agentstate

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestIntegrationMigrationV5PreservesExistingAgentData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	id := mustInstallationID(t, "11111111-2222-4333-8444-555555555555")
	if _, err := NewIdentityStore(path).EnsureInstallationID(id); err != nil {
		t.Fatal(err)
	}
	want := State{DesiredGeneration: 7, ActivatedGeneration: 6, AppliedGeneration: 5, Dependencies: map[string]DependencyState{"node-exporter": {Enabled: true}}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	legacyAttempt := []byte(`{"status":"pending","payload":{"attemptId":"legacy-attempt"}}`)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO enrollment_attempt(id,payload,updated_at) VALUES(1,?,CURRENT_TIMESTAMP)`, legacyAttempt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE integration_instances`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE integration_desired_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=6`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = NewSQLiteStore(path).open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var payload []byte
	if err := db.QueryRow(`SELECT payload FROM enrollment_attempt WHERE id=1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != string(legacyAttempt) {
		t.Fatalf("enrollment payload changed: %s", payload)
	}
	var initialized int
	var identityID string
	if err := db.QueryRow(`SELECT initialized,installation_id FROM installation_identity WHERE id=1`).Scan(&initialized, &identityID); err != nil {
		t.Fatal(err)
	}
	if initialized != 1 || identityID != id.String() {
		t.Fatalf("identity after migration = %d %s", initialized, identityID)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DesiredGeneration != want.DesiredGeneration || loaded.ActivatedGeneration != want.ActivatedGeneration || loaded.AppliedGeneration != want.AppliedGeneration || !loaded.Dependencies["node-exporter"].Enabled {
		t.Fatalf("M1/M2 state changed: %+v", loaded)
	}
}

func TestIntegrationSnapshotsStayConsistentDuringConcurrentCreates(t *testing.T) {
	repo := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db")).IntegrationRepository()
	const count = 20
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		for i := 0; i < count; i++ {
			if _, _, err := repo.Put(integrationInstance(string(rune('a'+i)), "localhost:6379"), 0); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	close(start)
	for i := 0; i < 100; i++ {
		snap, err := repo.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if uint64(len(snap.Instances)) != snap.Revision {
			t.Fatalf("inconsistent snapshot revision=%d instances=%d", snap.Revision, len(snap.Instances))
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func integrationInstance(id, endpoint string) integration.IntegrationInstance {
	return integration.IntegrationInstance{ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": endpoint}}
}

func TestSQLiteIntegrationMutationsCASNoOpDeleteRestartAndMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	repo := store.IntegrationRepository()
	created, setRev, err := repo.Put(integrationInstance("cache", "localhost:6379"), 0)
	if err != nil || created.Revision != 1 || setRev != 1 {
		t.Fatalf("create = rev %d set %d err %v", created.Revision, setRev, err)
	}
	noop, setRev, err := repo.Put(integrationInstance("cache", "localhost:6379"), 1)
	if err != nil || noop.Revision != 1 || setRev != 1 {
		t.Fatalf("no-op = rev %d set %d err %v", noop.Revision, setRev, err)
	}
	if _, _, err := repo.Put(integrationInstance("cache", "other:6379"), 0); !errors.Is(err, configuration.ErrRevisionConflict) {
		t.Fatalf("stale CAS error = %v", err)
	}
	updated, setRev, err := repo.Put(integrationInstance("cache", "other:6379"), 1)
	if err != nil || updated.Revision != 2 || setRev != 2 {
		t.Fatalf("update = rev %d set %d err %v", updated.Revision, setRev, err)
	}
	restarted := NewSQLiteStore(path).IntegrationRepository()
	snapshot, err := restarted.Snapshot()
	if err != nil || snapshot.Revision != 2 || len(snapshot.Instances) != 1 || snapshot.Instances[0].Revision != 2 {
		t.Fatalf("snapshot = %+v err %v", snapshot, err)
	}
	setRev, err = restarted.Delete("cache", 2)
	if err != nil || setRev != 3 {
		t.Fatalf("delete set revision %d err %v", setRev, err)
	}
	setRev, err = restarted.Delete("cache", 2)
	if err != nil || setRev != 3 {
		t.Fatalf("repeat delete set revision %d err %v", setRev, err)
	}
	state, err := store.Load()
	if err == nil && state.DesiredGeneration != 0 {
		t.Fatalf("integration mutation changed lifecycle generation: %d", state.DesiredGeneration)
	}
}

func TestSQLiteIntegrationLegacySourceNormalizesWithoutAdvancingRevision(t *testing.T) {
	repository := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db")).IntegrationRepository()
	legacy := integrationInstance("legacy-cache", "localhost:6379")
	created, setRevision, err := repository.Put(legacy, 0)
	if err != nil || setRevision != 1 || created.Revision != 1 {
		t.Fatalf("legacy create=%+v desired revision=%d err=%v", created, setRevision, err)
	}
	snapshot, err := repository.Snapshot()
	if err != nil || snapshot.Instances[0].Source != integration.SourceLocal {
		t.Fatalf("legacy source snapshot=%+v err=%v", snapshot, err)
	}
	explicitLocal := legacy
	explicitLocal.Source = integration.SourceLocal
	unchanged, setRevision, err := repository.Put(explicitLocal, 1)
	if err != nil || unchanged.Revision != 1 || setRevision != 1 {
		t.Fatalf("normalizing legacy source advanced revision: instance=%+v desired=%d err=%v", unchanged, setRevision, err)
	}
}

func TestSQLiteIntegrationMutationRollsBackInstanceWhenMarkerUpdateFails(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repo := store.IntegrationRepository()
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_marker BEFORE UPDATE ON integration_desired_state BEGIN SELECT RAISE(ABORT,'injected marker failure'); END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, _, err := repo.Put(integrationInstance("cache", "localhost:6379"), 0); err == nil {
		t.Fatal("Put succeeded despite injected marker failure")
	}
	snapshot, err := repo.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 || len(snapshot.Instances) != 0 {
		t.Fatalf("partial mutation survived rollback: %+v", snapshot)
	}
}

func TestSQLiteIntegrationConcurrentCreatesAdvanceMarkerWithoutLostWrites(t *testing.T) {
	repo := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db")).IntegrationRepository()
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := repo.Put(integrationInstance(string(rune('a'+i)), "localhost:6379"), 0)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := repo.Snapshot()
	if err != nil || snapshot.Revision != 12 || len(snapshot.Instances) != 12 {
		t.Fatalf("snapshot = rev %d count %d err %v", snapshot.Revision, len(snapshot.Instances), err)
	}
}

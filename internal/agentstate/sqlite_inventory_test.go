package agentstate

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func inventoryFixture(at time.Time, ids ...string) (discovery.ProviderSnapshot, []discovery.Resource) {
	snapshot := discovery.ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at}
	resources := make([]discovery.Resource, 0, len(ids))
	for _, id := range ids {
		observation := discovery.ResourceObservation{ProviderResourceID: id, Kind: discovery.ResourceContainer, DisplayName: id, LifecycleState: "running", Attributes: map[string]string{"label.safe": "value"}, Endpoints: []discovery.Endpoint{{Address: "10.0.0.2", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"}}}
		snapshot.Resources = append(snapshot.Resources, observation)
		resources = append(resources, discovery.Resource{Key: discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: id}, Kind: observation.Kind, DisplayName: id, LifecycleState: "running", Attributes: map[string]string{"label.safe": "value"}, Endpoints: append([]discovery.Endpoint(nil), observation.Endpoints...), Presence: discovery.PresencePresent, FirstSeenAt: at, LastSeenAt: at})
	}
	return snapshot, resources
}

func TestSQLiteInventoryRevisionCASNoopFailureAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	repository := store.InventoryRepository()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	snapshot, resources := inventoryFixture(at, "container-a")
	if err := repository.CommitProviderSnapshot(0, snapshot, resources, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 || loaded.WriteRevision != 1 || len(loaded.Resources) != 1 || loaded.Resources[0].Key.ProviderResourceID != "container-a" {
		t.Fatalf("initial snapshot=%+v", loaded)
	}
	if err := repository.CommitProviderSnapshot(0, snapshot, resources, true); !errors.Is(err, discovery.ErrSnapshotConflict) {
		t.Fatalf("stale CAS error=%v", err)
	}
	snapshot.ObservedAt = at.Add(time.Minute)
	resources[0].LastSeenAt = snapshot.ObservedAt
	if err := repository.CommitProviderSnapshot(loaded.WriteRevision, snapshot, resources, false); err != nil {
		t.Fatal(err)
	}
	loaded, err = repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 || loaded.WriteRevision != 2 || !loaded.Resources[0].LastSeenAt.Equal(snapshot.ObservedAt) {
		t.Fatalf("no-op revision or last_seen incorrect: %+v", loaded)
	}
	failure := discovery.ProviderStatus{ProviderID: "docker", ProviderInstanceID: "local", Enabled: true, LastAttemptAt: at.Add(2 * time.Minute), LastErrorCode: "daemon_unavailable"}
	if err := repository.RecordProviderFailure(failure); err != nil {
		t.Fatal(err)
	}
	loaded, err = repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 || loaded.WriteRevision != 3 || len(loaded.Resources) != 1 || loaded.Resources[0].Presence != discovery.PresencePresent || loaded.Providers[0].LastErrorCode != "daemon_unavailable" || !loaded.Providers[0].LastSuccessAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("failure changed resource evidence or lost success: %+v", loaded)
	}
	restarted, err := NewSQLiteStore(path).InventoryRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Revision != loaded.Revision || restarted.Resources[0].Key != loaded.Resources[0].Key || restarted.Providers[0].LastErrorCode != loaded.Providers[0].LastErrorCode {
		t.Fatalf("restart lost inventory: %+v", restarted)
	}
}

func TestSQLiteInventoryCASRejectsSnapshotMadeStaleByNonMaterialWrites(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repo := store.InventoryRepository()
	at := time.Now().UTC()
	snapshot, resources := inventoryFixture(at, "container-a")
	if err := repo.CommitProviderSnapshot(0, snapshot, resources, true); err != nil {
		t.Fatal(err)
	}
	stale, err := repo.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordProviderFailure(discovery.ProviderStatus{
		ProviderID: "docker", ProviderInstanceID: "local", Enabled: true,
		LastAttemptAt: at.Add(time.Second), LastErrorCode: "daemon_unavailable",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitProviderSnapshot(stale.WriteRevision, snapshot, resources, false); !errors.Is(err, discovery.ErrSnapshotConflict) {
		t.Fatalf("stale non-material writer error=%v, want snapshot conflict", err)
	}
	current, err := repo.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != stale.Revision || current.Providers[0].LastErrorCode != "daemon_unavailable" {
		t.Fatalf("non-material write changed material evidence or was lost: %+v", current)
	}
}

func TestMigrateInventoryWriteRevisionFromPopulatedV10PreservesInventory(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`INSERT INTO schema_migrations(version,applied_at) VALUES(10,'before')`,
		`CREATE TABLE inventory_meta(id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL CHECK(revision>=0))`,
		`INSERT INTO inventory_meta(id,revision) VALUES(1,7)`,
		`CREATE TABLE inventory_resources(provider_id TEXT NOT NULL,provider_instance_id TEXT NOT NULL,provider_resource_id TEXT NOT NULL,payload BLOB NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(provider_id,provider_instance_id,provider_resource_id))`,
		`INSERT INTO inventory_resources VALUES('docker','local','resource-1','{"key":"preserve"}','before')`,
		`CREATE TABLE discovery_provider_status(provider_id TEXT NOT NULL,provider_instance_id TEXT NOT NULL,payload BLOB NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(provider_id,provider_instance_id))`,
		`INSERT INTO discovery_provider_status VALUES('docker','local','{"status":"preserve"}','before')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateInventoryWriteRevision(db); err != nil {
		t.Fatal(err)
	}
	var materialRevision, writeRevision int
	if err := db.QueryRow(`SELECT revision,write_revision FROM inventory_meta WHERE id=1`).Scan(&materialRevision, &writeRevision); err != nil {
		t.Fatal(err)
	}
	var resourcePayload, statusPayload string
	if err := db.QueryRow(`SELECT payload FROM inventory_resources WHERE provider_resource_id='resource-1'`).Scan(&resourcePayload); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT payload FROM discovery_provider_status WHERE provider_id='docker'`).Scan(&statusPayload); err != nil {
		t.Fatal(err)
	}
	var schemaVersion int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if materialRevision != 7 || writeRevision != 7 || resourcePayload != `{"key":"preserve"}` || statusPayload != `{"status":"preserve"}` || schemaVersion != 11 {
		t.Fatalf("migration changed existing inventory: material=%d write=%d resource=%q status=%q version=%d", materialRevision, writeRevision, resourcePayload, statusPayload, schemaVersion)
	}
}

func TestSQLiteInventoryDeleteEvidenceAndAtomicRollback(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repo := store.InventoryRepository()
	at := time.Now().UTC()
	snapshot, resources := inventoryFixture(at, "keep")
	if err := repo.CommitProviderSnapshot(0, snapshot, resources, true); err != nil {
		t.Fatal(err)
	}
	missing := discovery.ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at.Add(time.Minute)}
	next, changed, err := discovery.ReconcileProviderSnapshot(resources, missing, discovery.Policy{MissingAfterSnapshots: 1, StaleAfter: time.Minute})
	if err != nil || !changed {
		t.Fatalf("reconcile missing changed=%v err=%v", changed, err)
	}
	if err := repo.CommitProviderSnapshot(1, missing, next, true); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_provider_status BEFORE INSERT ON discovery_provider_status BEGIN SELECT RAISE(ABORT,'injected rollback'); END`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	second, added := inventoryFixture(at.Add(2*time.Minute), "new")
	err = repo.CommitProviderSnapshot(before.WriteRevision, second, added, true)
	if err == nil {
		t.Fatal("injected persistence failure unexpectedly committed")
	}
	after, err := repo.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.Resources) != len(before.Resources) || after.Resources[0].Presence != before.Resources[0].Presence {
		t.Fatalf("rollback left partial inventory: before=%+v after=%+v", before, after)
	}
}

func TestSQLiteInventoryConcurrentCASAndConsistentSnapshots(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repo := store.InventoryRepository()
	const count = 24
	start := make(chan struct{})
	stopReader := make(chan struct{})
	writerErrors := make(chan error, count)
	readerErrors := make(chan error, 1)
	var writers sync.WaitGroup

	for i := 0; i < count; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			<-start
			instance := fmt.Sprintf("local-%02d", i)
			at := time.Now().UTC()
			snapshot, resources := inventoryFixtureForInstance(at, instance, fmt.Sprintf("resource-%02d", i))
			for attempt := 0; attempt < 100; attempt++ {
				current, err := repo.Snapshot()
				if err != nil {
					writerErrors <- err
					return
				}
				err = repo.CommitProviderSnapshot(current.WriteRevision, snapshot, resources, true)
				if err == nil {
					writerErrors <- nil
					return
				}
				if !errors.Is(err, discovery.ErrSnapshotConflict) {
					writerErrors <- err
					return
				}
				time.Sleep(time.Millisecond)
			}
			writerErrors <- fmt.Errorf("writer %d exhausted CAS retries", i)
		}(i)
	}

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			snapshot, err := repo.Snapshot()
			if err != nil {
				readerErrors <- err
				return
			}
			if snapshot.Revision != uint64(len(snapshot.Resources)) {
				readerErrors <- fmt.Errorf("inconsistent snapshot revision=%d resources=%d", snapshot.Revision, len(snapshot.Resources))
				return
			}
			select {
			case <-stopReader:
				return
			default:
			}
			time.Sleep(time.Millisecond)
		}
	}()

	close(start)
	writers.Wait()
	close(stopReader)
	<-readerDone
	close(writerErrors)
	for err := range writerErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-readerErrors:
		t.Fatal(err)
	default:
	}

	final, err := repo.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != count || len(final.Resources) != count {
		t.Fatalf("final rev=%d resources=%d, want %d", final.Revision, len(final.Resources), count)
	}
}

func inventoryFixtureForInstance(at time.Time, instance string, ids ...string) (discovery.ProviderSnapshot, []discovery.Resource) {
	snapshot, resources := inventoryFixture(at, ids...)
	snapshot.ProviderInstanceID = discovery.ProviderInstanceID(instance)
	for i := range resources {
		resources[i].Key.ProviderInstance = discovery.ProviderInstanceID(instance)
	}
	return snapshot, resources
}

func TestSQLiteInventoryMigrationFromPopulatedV9PreservesExistingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	state := State{DesiredGeneration: 8, ActivatedGeneration: 7, AppliedGeneration: 7, Dependencies: map[string]DependencyState{"host": {Enabled: true}}}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	installationID, err := identity.ParseInstallationID("123e4567-e89b-12d3-a456-426614174000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureInstallationID(installationID); err != nil {
		t.Fatal(err)
	}
	instance := integration.IntegrationInstance{ID: "redis-main", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}}
	if _, _, err := store.IntegrationRepository().Put(instance, 0); err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO enrollment_attempt(id,payload,updated_at) VALUES(1,'fixture','before-v10')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE discovery_provider_status; DROP TABLE inventory_resources; DROP TABLE inventory_meta; DROP TABLE integration_proposals; DELETE FROM schema_migrations WHERE version IN (10,11,12)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	restarted := NewSQLiteStore(path)
	snapshot, err := restarted.InventoryRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 || len(snapshot.Resources) != 0 {
		t.Fatalf("new inventory not initialized: %+v", snapshot)
	}
	loaded, err := restarted.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DesiredGeneration != 8 || loaded.ActivatedGeneration != 7 || loaded.AppliedGeneration != 7 || !loaded.Dependencies["host"].Enabled {
		t.Fatalf("M1/M2 state changed: %+v", loaded)
	}
	desired, err := restarted.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if desired.Revision != 1 || len(desired.Instances) != 1 || desired.Instances[0].ID != "redis-main" {
		t.Fatalf("desired integrations changed: %+v", desired)
	}
	verify, err := restarted.open()
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var identityValue, enrollmentRows, version int
	if err := verify.QueryRow(`SELECT COUNT(*) FROM installation_identity WHERE initialized=1`).Scan(&identityValue); err != nil {
		t.Fatal(err)
	}
	if err := verify.QueryRow(`SELECT COUNT(*) FROM enrollment_attempt`).Scan(&enrollmentRows); err != nil {
		t.Fatal(err)
	}
	if err := verify.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if identityValue != 1 || enrollmentRows != 1 || version != 13 {
		t.Fatalf("identity=%d enrollment=%d migration=%d", identityValue, enrollmentRows, version)
	}
}

package agentstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestSQLiteTemplateRepositoryCASNoOpDeleteTombstoneAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	repository := NewSQLiteStore(path).TemplateRepository()
	created, setRevision, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateEnabled), 0)
	if err != nil || created.Revision != 1 || setRevision != 1 {
		t.Fatalf("create template=%+v set revision=%d err=%v", created, setRevision, err)
	}
	unchanged, setRevision, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateEnabled), 1)
	if err != nil || unchanged.Revision != 1 || setRevision != 1 {
		t.Fatalf("no-op template=%+v set revision=%d err=%v", unchanged, setRevision, err)
	}
	if _, _, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateDisabled), 0); !errors.Is(err, configuration.ErrRevisionConflict) {
		t.Fatalf("stale CAS error=%v", err)
	}
	updated, setRevision, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateDisabled), 1)
	if err != nil || updated.Revision != 2 || setRevision != 2 {
		t.Fatalf("update template=%+v set revision=%d err=%v", updated, setRevision, err)
	}
	changedTarget := updated
	changedTarget.DefinitionID = "postgres"
	if _, _, err := repository.Put(changedTarget, 2); !errors.Is(err, configuration.ErrTemplateTargetImmutable) {
		t.Fatalf("changing a Template's Integration target error=%v", err)
	}
	tombstone := sqliteTestTemplate("redis-main", integration.TemplateDeleted)
	tombstone.Revision = 2
	deleted, setRevision, err := repository.Put(tombstone, 2)
	if err != nil || deleted.Revision != 3 || setRevision != 3 {
		t.Fatalf("delete template=%+v set revision=%d err=%v", deleted, setRevision, err)
	}
	if _, _, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateEnabled), 3); !errors.Is(err, configuration.ErrTemplateDeleted) {
		t.Fatalf("tombstone reuse error=%v", err)
	}
	restarted := NewSQLiteStore(path).TemplateRepository()
	snapshot, err := restarted.Snapshot()
	if err != nil || snapshot.Revision != 3 || len(snapshot.Templates) != 1 || snapshot.Templates[0].Status != integration.TemplateDeleted {
		t.Fatalf("restart snapshot=%+v err=%v", snapshot, err)
	}
}

func TestSQLiteTemplateMutationRollbackAndMigrationCompatibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	if _, err := NewIdentityStore(path).EnsureInstallationID(mustInstallationID(t, "21111111-2222-4333-8444-555555555555")); err != nil {
		t.Fatal(err)
	}
	state := State{DesiredGeneration: 6, ActivatedGeneration: 5, AppliedGeneration: 4, Dependencies: map[string]DependencyState{"node-exporter": {Enabled: true}}}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	repository := store.TemplateRepository()
	if _, _, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateEnabled), 0); err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_template_revision BEFORE UPDATE ON integration_template_state BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, _, err := repository.Put(sqliteTestTemplate("redis-main", integration.TemplateDisabled), 1); err == nil {
		t.Fatal("mutation succeeded despite injected set revision failure")
	}
	snapshot, err := repository.Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Templates) != 1 || snapshot.Templates[0].Status != integration.TemplateEnabled || snapshot.Templates[0].Revision != 1 {
		t.Fatalf("rollback snapshot=%+v err=%v", snapshot, err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.DesiredGeneration != state.DesiredGeneration || loaded.ActivatedGeneration != state.ActivatedGeneration || loaded.AppliedGeneration != state.AppliedGeneration || !loaded.Dependencies["node-exporter"].Enabled {
		t.Fatalf("M1/M2 state changed during migration/use: %+v err=%v", loaded, err)
	}
	var version int
	if err := dbQueryVersion(t, path, &version); err != nil || version != 13 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestSQLiteTemplateMigrationFromV12PreservesPopulatedOperationalTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	installationID := mustInstallationID(t, "31111111-2222-4333-8444-555555555555")
	if _, err := NewIdentityStore(path).EnsureInstallationID(installationID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.IntegrationRepository().Put(integrationInstance("legacy-cache", "localhost:6379"), 0); err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	state := State{DesiredGeneration: 9, ActivatedGeneration: 8, AppliedGeneration: 7, Dependencies: map[string]DependencyState{"node-exporter": {Enabled: true}}}
	encodedState, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_state(id,payload,updated_at) VALUES(1,?,CURRENT_TIMESTAMP)`, encodedState); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO enrollment_attempt(id,payload,updated_at) VALUES(1,?,CURRENT_TIMESTAMP)`, []byte(`{"status":"pending"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO activation_journal(generation,outcome,updated_at) VALUES(7,'applied',CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO inventory_resources(provider_id,provider_instance_id,provider_resource_id,payload,updated_at) VALUES('docker','local','container-a',?,CURRENT_TIMESTAMP)`, []byte(`{"id":"container-a"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO integration_proposals(proposal_id,provider_id,provider_instance_id,resource_id,integration_id,revision,status,payload,updated_at) VALUES('proposal-a','docker','local','container-a','redis',1,'pending',?,CURRENT_TIMESTAMP)`, []byte(`{"id":"proposal-a"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE integration_templates; DROP TABLE integration_template_state; DELETE FROM schema_migrations WHERE version=13`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	reopened, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for table, want := range map[string]int{"agent_state": 1, "enrollment_attempt": 1, "activation_journal": 1, "inventory_resources": 1, "integration_proposals": 1, "integration_instances": 1} {
		var count int
		if err := reopened.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	loaded, err := store.Load()
	if err != nil || loaded.DesiredGeneration != 9 || loaded.ActivatedGeneration != 8 || loaded.AppliedGeneration != 7 || !loaded.Dependencies["node-exporter"].Enabled {
		t.Fatalf("M1/M2 state after v13 migration=%+v err=%v", loaded, err)
	}
	var identityValue string
	var initialized int
	if err := reopened.QueryRow(`SELECT initialized,installation_id FROM installation_identity WHERE id=1`).Scan(&initialized, &identityValue); err != nil || initialized != 1 || identityValue != installationID.String() {
		t.Fatalf("Installation Identity after migration initialized=%d id=%s err=%v", initialized, identityValue, err)
	}
	var maxVersion int
	if err := reopened.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&maxVersion); err != nil || maxVersion != 13 {
		t.Fatalf("schema version=%d err=%v", maxVersion, err)
	}
}

func TestSQLiteTemplateSnapshotsRemainConsistentDuringConcurrentCreates(t *testing.T) {
	repository := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db")).TemplateRepository()
	const count = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, _, err := repository.Put(sqliteTestTemplate(fmt.Sprintf("redis-%02d", index), integration.TemplateEnabled), 0)
			errs <- err
		}(i)
	}
	close(start)
	for i := 0; i < 20; i++ {
		snapshot, err := repository.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Revision != uint64(len(snapshot.Templates)) {
			t.Fatalf("snapshot revision=%d templates=%d", snapshot.Revision, len(snapshot.Templates))
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	final, err := repository.Snapshot()
	if err != nil || final.Revision != count || len(final.Templates) != count {
		t.Fatalf("final snapshot revision=%d templates=%d err=%v", final.Revision, len(final.Templates), err)
	}
}

func TestSQLiteIntegrationRepositoryCannotMutateTemplateOwnedInstance(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := store.IntegrationRepository()
	instance := integration.IntegrationInstance{
		ID: "template-0123456789abcdef01234567", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Source: integration.SourceTemplate,
		TemplateOwner: integration.TemplateOwnerLocal, TemplateID: "redis-main", TemplateRevision: 1,
		Binding: &integration.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"}, Enabled: true,
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(instance)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO integration_instances(instance_id,payload,revision,updated_at) VALUES(?,?,1,CURRENT_TIMESTAMP)`, instance.ID, payload); err != nil {
		t.Fatal(err)
	}
	db.Close()
	localUpdate := instance
	localUpdate.Source = integration.SourceLocal
	localUpdate.Binding = nil
	localUpdate.TemplateOwner, localUpdate.TemplateID, localUpdate.TemplateRevision = "", "", 0
	if _, _, err := repository.Put(localUpdate, 1); !errors.Is(err, configuration.ErrTemplateOwnedInstance) {
		t.Fatalf("overwrite template instance error=%v", err)
	}
	if _, err := repository.Delete(instance.ID, 1); !errors.Is(err, configuration.ErrTemplateOwnedInstance) {
		t.Fatalf("delete template instance error=%v", err)
	}
}

func sqliteTestTemplate(id string, status integration.TemplateStatus) integration.IntegrationTemplate {
	return integration.IntegrationTemplate{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: integration.TemplateOwnerLocal,
		Revision: 1, Status: status, Settings: map[string]any{},
		Bindings: []integration.TemplateSettingBinding{{Target: "endpoint", Source: integration.TemplateBindingSource{
			Kind: integration.TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}

func dbQueryVersion(t *testing.T, path string, version *int) error {
	t.Helper()
	db, err := NewSQLiteStore(path).open()
	if err != nil {
		return err
	}
	defer db.Close()
	return db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(version)
}

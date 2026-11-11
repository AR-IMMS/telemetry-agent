package agentstate

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestSQLiteMaterializationAppliesTemplateProjectionIdempotently(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	repository := store.MaterializationRepository()
	instance := generatedTestInstance()
	batch := configuration.MaterializationBatch{
		ExpectedInventoryWriteRevision: 1,
		ExpectedTemplateSetRevision:    1,
		ExpectedDesiredSetRevision:     0,
		Upsert:                         []integration.IntegrationInstance{instance},
	}

	revision, err := repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 1 {
		t.Fatalf("ApplyTemplateProjection() revision=%d err=%v", revision, err)
	}
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 {
		t.Fatalf("desired snapshot=%+v err=%v", snapshot, err)
	}
	if got := snapshot.Instances[0]; got.Revision != 1 || got.Source != integration.SourceTemplate || !got.Enabled {
		t.Fatalf("generated instance=%+v", got)
	}

	batch.ExpectedDesiredSetRevision = revision
	revision, err = repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 1 {
		t.Fatalf("idempotent projection revision=%d err=%v", revision, err)
	}
	snapshot, err = store.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 1 || snapshot.Instances[0].Revision != 1 {
		t.Fatalf("no-op advanced desired state: snapshot=%+v err=%v", snapshot, err)
	}

	batch.Upsert = nil
	batch.Suspend = []string{instance.ID}
	batch.ExpectedDesiredSetRevision = 1
	revision, err = repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 2 {
		t.Fatalf("suspend revision=%d err=%v", revision, err)
	}
	snapshot, err = store.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 2 || len(snapshot.Instances) != 1 || snapshot.Instances[0].Enabled || snapshot.Instances[0].Revision != 2 {
		t.Fatalf("suspended snapshot=%+v err=%v", snapshot, err)
	}
}

func TestSQLiteMaterializationRejectsStaleSnapshotTokens(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, store SQLiteStore)
		batch  configuration.MaterializationBatch
	}{
		{name: "inventory", mutate: func(t *testing.T, store SQLiteStore) {
			execMaterializationSQL(t, store, `UPDATE inventory_meta SET write_revision=2 WHERE id=1`)
		}, batch: configuration.MaterializationBatch{ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1}},
		{name: "templates", mutate: func(t *testing.T, store SQLiteStore) {
			_, _, err := store.TemplateRepository().Put(sqliteTestTemplate("redis-main", integration.TemplateDisabled), 1)
			if err != nil {
				t.Fatal(err)
			}
		}, batch: configuration.MaterializationBatch{ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1}},
		{name: "desired", mutate: func(t *testing.T, store SQLiteStore) {
			if _, _, err := store.IntegrationRepository().Put(integrationInstance("local-cache", "localhost:6380"), 0); err != nil {
				t.Fatal(err)
			}
		}, batch: configuration.MaterializationBatch{ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			prepareMaterializationRevisions(t, store, 1, 0)
			test.mutate(t, store)
			test.batch.ExpectedDesiredSetRevision = 0
			test.batch.Upsert = []integration.IntegrationInstance{generatedTestInstance()}
			if _, err := store.MaterializationRepository().ApplyTemplateProjection(test.batch); !errors.Is(err, configuration.ErrMaterializationConflict) {
				t.Fatalf("stale snapshot error=%v", err)
			}
		})
	}
}

func TestSQLiteMaterializationRollsBackAllProjectionWrites(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	execMaterializationSQL(t, store, `CREATE TRIGGER reject_materialized_revision BEFORE UPDATE ON integration_desired_state BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	_, err := store.MaterializationRepository().ApplyTemplateProjection(configuration.MaterializationBatch{
		ExpectedInventoryWriteRevision: 1,
		ExpectedTemplateSetRevision:    1,
		ExpectedDesiredSetRevision:     0,
		Upsert:                         []integration.IntegrationInstance{generatedTestInstance()},
	})
	if err == nil {
		t.Fatal("projection succeeded despite desired revision trigger failure")
	}
	snapshot, snapshotErr := store.IntegrationRepository().Snapshot()
	if snapshotErr != nil || snapshot.Revision != 0 || len(snapshot.Instances) != 0 {
		t.Fatalf("partial projection survived rollback: snapshot=%+v err=%v", snapshot, snapshotErr)
	}
}

func TestSQLiteMaterializationSerializesConcurrentSnapshotWriters(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	instances := []integration.IntegrationInstance{generatedTestInstance(), generatedTestInstanceForResource("container-b")}
	start := make(chan struct{})
	results := make(chan error, len(instances))
	var workers sync.WaitGroup
	for _, instance := range instances {
		workers.Add(1)
		go func(instance integration.IntegrationInstance) {
			defer workers.Done()
			<-start
			_, err := store.MaterializationRepository().ApplyTemplateProjection(configuration.MaterializationBatch{
				ExpectedInventoryWriteRevision: 1,
				ExpectedTemplateSetRevision:    1,
				ExpectedDesiredSetRevision:     0,
				Upsert:                         []integration.IntegrationInstance{instance},
			})
			results <- err
		}(instance)
	}
	close(start)
	workers.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, configuration.ErrMaterializationConflict) {
			conflicted++
		} else {
			t.Fatalf("concurrent projection error=%v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("successful projections=%d conflicts=%d, want 1 each", succeeded, conflicted)
	}
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 {
		t.Fatalf("concurrent writer snapshot=%+v err=%v", snapshot, err)
	}
}

func prepareMaterializationRevisions(t *testing.T, store SQLiteStore, inventory, desired uint64) {
	t.Helper()
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE inventory_meta SET write_revision=? WHERE id=1`, inventory); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE integration_desired_state SET desired_set_revision=? WHERE id=1`, desired); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.TemplateRepository().Put(sqliteTestTemplate("redis-main", integration.TemplateEnabled), 0); err != nil {
		t.Fatal(err)
	}
}

func execMaterializationSQL(t *testing.T, store SQLiteStore, query string) {
	t.Helper()
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func generatedTestInstance() integration.IntegrationInstance {
	return generatedTestInstanceForResource("container-a")
}

func generatedTestInstanceForResource(resourceID string) integration.IntegrationInstance {
	id, _ := integration.GeneratedInstanceID(integration.TemplateOwnerLocal, "redis-main", integration.ResourceIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: resourceID}, "redis")
	return integration.IntegrationInstance{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Source: integration.SourceTemplate,
		Binding:       &integration.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: resourceID},
		TemplateOwner: integration.TemplateOwnerLocal, TemplateID: "redis-main", TemplateRevision: 1, Enabled: true,
	}
}

package agentstate

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	templatesdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
)

type desiredEventPublisherFunc func(events.DesiredConfigurationChanged) error

func (f desiredEventPublisherFunc) Publish(event events.DesiredConfigurationChanged) error {
	return f(event)
}

func TestSQLiteMaterializationPublishesOnlyCommittedDesiredRevision(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	var published []uint64
	publisher := desiredEventPublisherFunc(func(event events.DesiredConfigurationChanged) error {
		published = append(published, event.Revision)
		snapshot, err := newTestDesiredStateRepository(store).Snapshot()
		if err != nil {
			return err
		}
		if snapshot.Revision != event.Revision {
			t.Fatalf("event revision %d published before committed snapshot %d", event.Revision, snapshot.Revision)
		}
		return nil
	})
	repository := newTestMaterializationRepositoryWithEvents(store, publisher)
	batch := materializationdomain.Batch{
		ExpectedInventoryWriteRevision: 1,
		ExpectedTemplateSetRevision:    1,
		ExpectedDesiredSetRevision:     0,
		Upsert:                         []desiredstatedomain.Instance{generatedTestInstance()},
	}
	if revision, err := repository.ApplyTemplateProjection(batch); err != nil || revision != 1 {
		t.Fatalf("committed materialization revision=%d err=%v", revision, err)
	}
	batch.ExpectedDesiredSetRevision = 1
	if revision, err := repository.ApplyTemplateProjection(batch); err != nil || revision != 1 {
		t.Fatalf("no-op materialization revision=%d err=%v", revision, err)
	}
	batch.ExpectedInventoryWriteRevision = 2
	if _, err := repository.ApplyTemplateProjection(batch); !errors.Is(err, materializationdomain.ErrMaterializationConflict) {
		t.Fatalf("stale materialization error = %v, want conflict", err)
	}
	if len(published) != 1 || published[0] != 1 {
		t.Fatalf("published revisions = %v, want [1]", published)
	}
}

func TestSQLiteMaterializationQueueFailureDoesNotUndoCommittedWrite(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	publisher := desiredEventPublisherFunc(func(events.DesiredConfigurationChanged) error { return events.ErrQueueFull })
	repository := newTestMaterializationRepositoryWithEvents(store, publisher)
	batch := materializationdomain.Batch{
		ExpectedInventoryWriteRevision: 1,
		ExpectedTemplateSetRevision:    1,
		Upsert:                         []desiredstatedomain.Instance{generatedTestInstance()},
	}
	if revision, err := repository.ApplyTemplateProjection(batch); err != nil || revision != 1 {
		t.Fatalf("committed write failed because advisory queue was full: revision=%d err=%v", revision, err)
	}
	snapshot, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 {
		t.Fatalf("durable desired state snapshot=%+v err=%v", snapshot, err)
	}
}

func TestSQLiteMaterializationAppliesTemplateProjectionIdempotently(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	repository := newTestMaterializationRepository(store)
	instance := generatedTestInstance()
	batch := materializationdomain.Batch{
		ExpectedInventoryWriteRevision: 1,
		ExpectedTemplateSetRevision:    1,
		ExpectedDesiredSetRevision:     0,
		Upsert:                         []desiredstatedomain.Instance{instance},
	}

	revision, err := repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 1 {
		t.Fatalf("ApplyTemplateProjection() revision=%d err=%v", revision, err)
	}
	snapshot, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 {
		t.Fatalf("desired snapshot=%+v err=%v", snapshot, err)
	}
	if got := snapshot.Instances[0]; got.Revision != 1 || got.Source != desiredstatedomain.SourceTemplate || !got.Enabled {
		t.Fatalf("generated instance=%+v", got)
	}

	batch.ExpectedDesiredSetRevision = revision
	revision, err = repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 1 {
		t.Fatalf("idempotent projection revision=%d err=%v", revision, err)
	}
	snapshot, err = newTestDesiredStateRepository(store).Snapshot()
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
	snapshot, err = newTestDesiredStateRepository(store).Snapshot()
	if err != nil || snapshot.Revision != 2 || len(snapshot.Instances) != 1 || snapshot.Instances[0].Enabled || snapshot.Instances[0].Revision != 2 {
		t.Fatalf("suspended snapshot=%+v err=%v", snapshot, err)
	}
}

func TestSQLiteMaterializationRejectsStaleSnapshotTokens(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, store SQLiteStore)
		batch  materializationdomain.Batch
	}{
		{name: "inventory", mutate: func(t *testing.T, store SQLiteStore) {
			execMaterializationSQL(t, store, `UPDATE inventory_meta SET write_revision=2 WHERE id=1`)
		}, batch: materializationdomain.Batch{ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1}},
		{name: "templates", mutate: func(t *testing.T, store SQLiteStore) {
			_, _, err := newTestTemplateRepository(store).Put(sqliteTestTemplate("redis-main", templatesdomain.StatusDisabled), 1)
			if err != nil {
				t.Fatal(err)
			}
		}, batch: materializationdomain.Batch{ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1}},
		{name: "desired", mutate: func(t *testing.T, store SQLiteStore) {
			if _, _, err := newTestDesiredStateRepository(store).Put(integrationInstance("local-cache", "localhost:6380"), 0); err != nil {
				t.Fatal(err)
			}
		}, batch: materializationdomain.Batch{ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			prepareMaterializationRevisions(t, store, 1, 0)
			test.mutate(t, store)
			test.batch.ExpectedDesiredSetRevision = 0
			test.batch.Upsert = []desiredstatedomain.Instance{generatedTestInstance()}
			if _, err := newTestMaterializationRepository(store).ApplyTemplateProjection(test.batch); !errors.Is(err, materializationdomain.ErrMaterializationConflict) {
				t.Fatalf("stale snapshot error=%v", err)
			}
		})
	}
}

func TestSQLiteMaterializationRollsBackAllProjectionWrites(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	execMaterializationSQL(t, store, `CREATE TRIGGER reject_materialized_revision BEFORE UPDATE ON integration_desired_state BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	_, err := newTestMaterializationRepository(store).ApplyTemplateProjection(materializationdomain.Batch{
		ExpectedInventoryWriteRevision: 1,
		ExpectedTemplateSetRevision:    1,
		ExpectedDesiredSetRevision:     0,
		Upsert:                         []desiredstatedomain.Instance{generatedTestInstance()},
	})
	if err == nil {
		t.Fatal("projection succeeded despite desired revision trigger failure")
	}
	snapshot, snapshotErr := newTestDesiredStateRepository(store).Snapshot()
	if snapshotErr != nil || snapshot.Revision != 0 || len(snapshot.Instances) != 0 {
		t.Fatalf("partial projection survived rollback: snapshot=%+v err=%v", snapshot, snapshotErr)
	}
}

func TestSQLiteMaterializationSerializesConcurrentSnapshotWriters(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	prepareMaterializationRevisions(t, store, 1, 0)
	instances := []desiredstatedomain.Instance{generatedTestInstance(), generatedTestInstanceForResource("container-b")}
	start := make(chan struct{})
	results := make(chan error, len(instances))
	var workers sync.WaitGroup
	for _, instance := range instances {
		workers.Add(1)
		go func(instance desiredstatedomain.Instance) {
			defer workers.Done()
			<-start
			_, err := newTestMaterializationRepository(store).ApplyTemplateProjection(materializationdomain.Batch{
				ExpectedInventoryWriteRevision: 1,
				ExpectedTemplateSetRevision:    1,
				ExpectedDesiredSetRevision:     0,
				Upsert:                         []desiredstatedomain.Instance{instance},
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
		} else if errors.Is(err, materializationdomain.ErrMaterializationConflict) {
			conflicted++
		} else {
			t.Fatalf("concurrent projection error=%v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("successful projections=%d conflicts=%d, want 1 each", succeeded, conflicted)
	}
	snapshot, err := newTestDesiredStateRepository(store).Snapshot()
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
	if _, _, err := newTestTemplateRepository(store).Put(sqliteTestTemplate("redis-main", templatesdomain.StatusEnabled), 0); err != nil {
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

func generatedTestInstance() desiredstatedomain.Instance {
	return generatedTestInstanceForResource("container-a")
}

func generatedTestInstanceForResource(resourceID string) desiredstatedomain.Instance {
	binding := desiredstatedomain.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: resourceID}
	id, _ := materializationdomain.GeneratedInstanceID(desiredstatedomain.TemplateOwnerLocal, "redis-main", binding, "redis")
	return desiredstatedomain.Instance{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Source: desiredstatedomain.SourceTemplate,
		Binding:       &binding,
		TemplateOwner: desiredstatedomain.TemplateOwnerLocal, TemplateID: "redis-main", TemplateRevision: 1, Enabled: true,
	}
}

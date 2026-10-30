package agentstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
)

func TestCoreGatewayAcceptanceKeepsRevisionsSeparateFromGenerations(t *testing.T) {
	store, id := seededGatewayStore(t)
	first, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 10, Endpoint: "bootstrap:4317"})
	if err != nil || first.Generation != 4 || first.Changed {
		t.Fatalf("first acceptance = %+v, %v", first, err)
	}
	second, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 11, Endpoint: "gateway:4317"})
	if err != nil || second.Generation != 5 || !second.Changed {
		t.Fatalf("changed acceptance = %+v, %v", second, err)
	}
	third, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 12, Endpoint: "gateway:4317"})
	if err != nil || third.Generation != 5 || third.Changed || third.Revision != 12 {
		t.Fatalf("unchanged higher revision = %+v, %v", third, err)
	}
	repeated, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 12, Endpoint: "gateway:4317"})
	if err != nil || repeated != third {
		t.Fatalf("same revision acceptance = %+v, %v", repeated, err)
	}
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 10, Endpoint: "bootstrap:4317"}); !errors.Is(err, coreconfig.ErrStaleRevision) {
		t.Fatalf("stale revision error = %v", err)
	}
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 12, Endpoint: "other:4317"}); !errors.Is(err, coreconfig.ErrRevisionConflict) {
		t.Fatalf("conflicting revision error = %v", err)
	}
	rollback, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 13, Endpoint: "bootstrap:4317"})
	if err != nil || rollback.Generation != 6 || !rollback.Changed {
		t.Fatalf("new-revision rollback = %+v, %v", rollback, err)
	}
	state, err := store.Load()
	if err != nil || state.DesiredGeneration != 6 || state.AppliedGeneration != 3 || state.ActivatedGeneration != 4 {
		t.Fatalf("M1 generations = %+v, %v", state, err)
	}
}

func TestCoreGatewayAcceptanceWaitsForCollectorActivationLock(t *testing.T) {
	store, id := seededGatewayStore(t)
	configPath := filepath.Join(t.TempDir(), "collector.yaml")
	if _, err := store.Update(func(state *State) error {
		state.Collector.ConfigPath = configPath
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	lock, err := AcquireLifecycleLock(configPath + ".lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
			InstallationID: id,
			Revision:       1,
			Endpoint:       "core.example:4317",
		})
		done <- err
	}()
	select {
	case err := <-done:
		_ = lock.Close()
		t.Fatalf("snapshot acceptance passed activation lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("accept after lock release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot acceptance did not resume after lock release")
	}
}

func TestCoreGatewayRuntimeMigrationPreservesActivatedButUnappliedRevision(t *testing.T) {
	store, installationID := seededGatewayStore(t)
	accepted, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
		InstallationID: installationID,
		Revision:       7,
		Endpoint:       "core.example:4317",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error {
		state.ActivatedGeneration = accepted.Generation
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE core_gateway_runtime`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 4`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	db, err = store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var endpoint string
	var activatedRevision, activatedGeneration, appliedRevision, appliedGeneration int64
	if err := db.QueryRow(`SELECT last_good_endpoint, activated_revision, activated_generation, applied_revision, applied_generation FROM core_gateway_runtime WHERE id = 1`).
		Scan(&endpoint, &activatedRevision, &activatedGeneration, &appliedRevision, &appliedGeneration); err != nil {
		t.Fatal(err)
	}
	if endpoint != "bootstrap:4317" || activatedRevision != 7 || activatedGeneration != int64(accepted.Generation) || appliedRevision != 0 || appliedGeneration != 3 {
		t.Fatalf("migrated runtime mapping = %q %d/%d %d/%d", endpoint, activatedRevision, activatedGeneration, appliedRevision, appliedGeneration)
	}
}

func TestCoreGatewayOwnershipAndBootstrapRerun(t *testing.T) {
	store, id := seededGatewayStore(t)
	before, err := store.EffectiveGateway()
	if err != nil || before.Owner != coreconfig.BootstrapOwner || before.Endpoint != "bootstrap:4317" {
		t.Fatalf("bootstrap gateway = %+v, %v", before, err)
	}
	for _, endpoint := range []string{"", "https://gateway:4317", "gateway:0", "gateway"} {
		if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 1, Endpoint: endpoint}); err == nil {
			t.Fatalf("invalid endpoint %q accepted", endpoint)
		}
	}
	afterInvalid, err := store.EffectiveGateway()
	if err != nil || afterInvalid != before {
		t.Fatalf("invalid snapshot changed owner: %+v, %v", afterInvalid, err)
	}
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 1, Endpoint: "core:4317"}); err != nil {
		t.Fatalf("accept Core snapshot: %v", err)
	}
	if _, err := store.Update(func(state *State) error {
		state.Collector.GatewayEndpoint = "new-bootstrap:4317"
		return nil
	}); err != nil {
		t.Fatalf("bootstrap rerun: %v", err)
	}
	effective, err := NewSQLiteStore(store.path).EffectiveGateway()
	if err != nil || effective.Owner != coreconfig.CoreOwner || effective.Endpoint != "core:4317" || effective.Revision != 1 {
		t.Fatalf("effective Core gateway after restart = %+v, %v", effective, err)
	}
}

func TestCoreGatewayConcurrentLocalUpdatePreservesM1State(t *testing.T) {
	store, id := seededGatewayStore(t)
	other := NewSQLiteStore(store.path)
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 1, Endpoint: "core:4317"})
		results <- err
	}()
	go func() {
		defer wait.Done()
		_, err := other.Update(func(state *State) error {
			_, err := state.EnableDependency("postgresql")
			return err
		})
		results <- err
	}()
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent operation: %v", err)
		}
	}
	state, err := store.Load()
	if err != nil || state.DesiredGeneration != 6 || !state.Dependencies["postgresql"].Enabled || state.Dependencies["redis"].Ownership.Resources[0].Identifier != "redis-service" {
		t.Fatalf("concurrent state = %+v, %v", state, err)
	}
	effective, err := store.EffectiveGateway()
	if err != nil || effective.Endpoint != "core:4317" {
		t.Fatalf("concurrent Core snapshot = %+v, %v", effective, err)
	}
}

func TestCoreGatewayConcurrentConflictingSnapshotsAcceptOnlyOne(t *testing.T) {
	store, id := seededGatewayStore(t)
	other := NewSQLiteStore(store.path)
	var wait sync.WaitGroup
	type outcome struct {
		accepted coreconfig.Acceptance
		err      error
	}
	results := make(chan outcome, 2)
	for index, repository := range []SQLiteStore{store, other} {
		endpoint := "first:4317"
		if index == 1 {
			endpoint = "second:4317"
		}
		wait.Add(1)
		go func(repository SQLiteStore, endpoint string) {
			defer wait.Done()
			accepted, err := repository.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 1, Endpoint: endpoint})
			results <- outcome{accepted: accepted, err: err}
		}(repository, endpoint)
	}
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			if result.accepted.Generation != 5 {
				t.Fatalf("accepted generation = %d, want 5", result.accepted.Generation)
			}
		case errors.Is(result.err, coreconfig.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent acceptance error: %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("outcomes: %d successes and %d conflicts, want one each", successes, conflicts)
	}
	state, err := store.Load()
	if err != nil || state.DesiredGeneration != 5 {
		t.Fatalf("concurrent desired generation = %+v, %v", state, err)
	}
}

func TestCoreGatewaySnapshotWriteFailureKeepsBootstrapOwnership(t *testing.T) {
	store, id := seededGatewayStore(t)
	db, err := store.open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_core_gateway BEFORE INSERT ON core_gateway_snapshot BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
	db.Close()
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: id, Revision: 1, Endpoint: "core:4317"}); err == nil {
		t.Fatal("acceptance succeeded despite failed snapshot write")
	}
	state, err := store.Load()
	if err != nil || state.DesiredGeneration != 4 || state.Dependencies["redis"].Ownership.Resources[0].Identifier != "redis-service" {
		t.Fatalf("failed acceptance changed M1 state = %+v, %v", state, err)
	}
	effective, err := store.EffectiveGateway()
	if err != nil || effective.Owner != coreconfig.BootstrapOwner || effective.Endpoint != "bootstrap:4317" {
		t.Fatalf("failed acceptance changed owner = %+v, %v", effective, err)
	}
}

func TestCoreGatewayRequiresMatchingInitializedInstallation(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err := store.Save(State{Collector: CollectorContext{GatewayEndpoint: "bootstrap:4317"}}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	snapshot := coreconfig.GatewaySnapshot{InstallationID: "11111111-2222-4333-8444-555555555555", Revision: 1, Endpoint: "core:4317"}
	if _, err := store.AcceptGatewaySnapshot(snapshot); err == nil {
		t.Fatal("uninitialized installation accepted Core snapshot")
	}
	if _, err := store.EnsureInstallationID(mustInstallationID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")); err != nil {
		t.Fatalf("initialize installation: %v", err)
	}
	if _, err := store.AcceptGatewaySnapshot(snapshot); err == nil {
		t.Fatal("different installation accepted Core snapshot")
	}
	if got, err := store.EffectiveGateway(); err != nil || got.Owner != coreconfig.BootstrapOwner {
		t.Fatalf("identity rejection changed owner = %+v, %v", got, err)
	}
}

func TestCoreGatewayLegacyImportAndRestart(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "state.json")
	want := State{Collector: CollectorContext{GatewayEndpoint: "legacy:4317"}, DesiredGeneration: 2,
		Dependencies: map[string]DependencyState{"redis": {Enabled: true}}}
	if err := NewFileStore(legacyPath).Save(want); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}
	repository := NewCoreGatewayStore(legacyPath)
	if _, err := NewIdentityStore(legacyPath).EnsureInstallationID(mustInstallationID(t, "11111111-2222-4333-8444-555555555555")); err != nil {
		t.Fatalf("initialize imported installation: %v", err)
	}
	accepted, err := repository.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: "11111111-2222-4333-8444-555555555555", Revision: 3, Endpoint: "core:4317"})
	if err != nil || accepted.Generation != 3 {
		t.Fatalf("accept after import = %+v, %v", accepted, err)
	}
	restarted := NewCoreGatewayStore(legacyPath)
	got, err := restarted.EffectiveGateway()
	if err != nil || got.Owner != coreconfig.CoreOwner || got.Endpoint != "core:4317" || got.Generation != 3 {
		t.Fatalf("restart effective Gateway = %+v, %v", got, err)
	}
	state, err := NewStore(legacyPath).Load()
	if err != nil || !state.Dependencies["redis"].Enabled || state.DesiredGeneration != 3 {
		t.Fatalf("imported M1 state = %+v, %v", state, err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}
}

func seededGatewayStore(t *testing.T) (SQLiteStore, string) {
	t.Helper()
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	want := State{Collector: CollectorContext{GatewayEndpoint: "bootstrap:4317"}, DesiredGeneration: 4, ActivatedGeneration: 4, AppliedGeneration: 3, Dependencies: map[string]DependencyState{"redis": {Enabled: true, Ownership: &OwnershipRecord{Resources: []OwnedResource{{Kind: "service", Identifier: "redis-service"}}}}}}
	if err := store.Save(want); err != nil {
		t.Fatalf("seed M1 state: %v", err)
	}
	id := "11111111-2222-4333-8444-555555555555"
	committed, err := store.EnsureInstallationID(mustInstallationID(t, id))
	if err != nil || committed.String() != id {
		t.Fatalf("seed installation ID = %q, %v", committed, err)
	}
	got, err := store.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("identity changed M1 state = %+v, %v", got, err)
	}
	return store, id
}

package agentstate

import (
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
)

func TestUnchangedCoreRevisionWaitsForRuntimeReadinessBeforeAppliedMapping(t *testing.T) {
	store, installationID := seededGatewayStore(t)
	first, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
		InstallationID: installationID,
		Revision:       10,
		Endpoint:       "bootstrap:4317",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkGatewayApplied(first.Generation, "bootstrap:4317"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
		InstallationID: installationID,
		Revision:       11,
		Endpoint:       "bootstrap:4317",
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := store.GatewayForGeneration(first.Generation)
	if err != nil || !pending.NeedsReadiness || pending.Revision != 11 {
		t.Fatalf("unchanged newer revision readiness state = %+v, %v", pending, err)
	}
	var appliedRevision uint64
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT applied_revision FROM core_gateway_runtime WHERE id = 1`).Scan(&appliedRevision); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if appliedRevision != 10 {
		t.Fatalf("applied Core revision = %d, want prior ready revision 10", appliedRevision)
	}
	if err := store.MarkGatewayApplied(first.Generation, "bootstrap:4317"); err != nil {
		t.Fatal(err)
	}
	ready, err := store.GatewayForGeneration(first.Generation)
	if err != nil || ready.NeedsReadiness || ready.AppliedRevision != 11 {
		t.Fatalf("readiness-confirmed revision state = %+v, %v", ready, err)
	}
	db, err = store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT applied_revision FROM core_gateway_runtime WHERE id = 1`).Scan(&appliedRevision); err != nil {
		t.Fatal(err)
	}
	if appliedRevision != 11 {
		t.Fatalf("applied Core revision after readiness = %d, want 11", appliedRevision)
	}
}

func TestNewerAcceptedGatewayDoesNotFailOlderCompletedActivationMapping(t *testing.T) {
	store, installationID := seededGatewayStore(t)
	first, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
		InstallationID: installationID,
		Revision:       10,
		Endpoint:       "first.example:4317",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error {
		return state.MarkGenerationActivated(first.Generation)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
		InstallationID: installationID,
		Revision:       11,
		Endpoint:       "second.example:4317",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkGatewayActivated(first.Generation); err != nil {
		t.Fatalf("record older completed activation while newer desired state is pending: %v", err)
	}
	var activatedRevision int64
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT activated_revision FROM core_gateway_runtime WHERE id = 1`).Scan(&activatedRevision); err != nil {
		t.Fatal(err)
	}
	if activatedRevision != 0 {
		t.Fatalf("activated Core revision = %d, want no mapping for superseded snapshot", activatedRevision)
	}
}

func TestGatewayRuntimeUsesLastGoodUntilCandidateGenerationActivates(t *testing.T) {
	store, installationID := seededGatewayStore(t)
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: installationID, Revision: 7, Endpoint: "core.example:4317"}); err != nil {
		t.Fatal(err)
	}
	before, err := store.GatewayForGeneration(4)
	if err != nil || before.Endpoint != "bootstrap:4317" || before.Revision != 0 {
		t.Fatalf("pre-activation Gateway = %+v, %v", before, err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkGenerationActivated(5) }); err != nil {
		t.Fatal(err)
	}
	candidate, err := store.GatewayForGeneration(5)
	if err != nil || candidate.Endpoint != "core.example:4317" || candidate.Revision != 7 || candidate.Generation != 5 {
		t.Fatalf("activated Gateway = %+v, %v", candidate, err)
	}
	if err := store.MarkGatewayApplied(5, candidate.Endpoint); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil || state.AppliedGeneration != 5 {
		t.Fatalf("applied state = %+v, %v", state, err)
	}
	committed, err := store.GatewayForGeneration(5)
	if err != nil || committed.Endpoint != candidate.Endpoint || committed.Revision != 7 || committed.Generation != 5 {
		t.Fatalf("committed Gateway = %+v, %v", committed, err)
	}
}

func TestGatewayRuntimeRetainsLastGoodEndpointForFailedCandidate(t *testing.T) {
	store, installationID := seededGatewayStore(t)
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: installationID, Revision: 7, Endpoint: "core.example:4317"}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkGatewayApplied(4, "bootstrap:4317"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkGenerationActivated(5) }); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkGatewayApplied(5, "invalid.example:4317"); err == nil {
		t.Fatal("MarkGatewayApplied() accepted endpoint that was not the active snapshot")
	}
	if _, err := store.Update(func(state *State) error { return state.RollbackUnappliedActivation(5) }); err != nil {
		t.Fatal(err)
	}
	got, err := store.GatewayForGeneration(4)
	if err != nil || got.Endpoint != "bootstrap:4317" {
		t.Fatalf("Gateway after rollback = %+v, %v", got, err)
	}
}

func TestCoreGatewayRuntimeMigrationSeedsExistingAcceptedSnapshot(t *testing.T) {
	store, installationID := seededGatewayStore(t)
	if _, err := store.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{InstallationID: installationID, Revision: 7, Endpoint: "core.example:4317"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error {
		state.AppliedGeneration = state.DesiredGeneration
		state.ActivatedGeneration = state.DesiredGeneration
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
	path := filepath.Clean(store.path)
	if _, err := NewSQLiteStore(path).Load(); err != nil {
		t.Fatal(err)
	}
	got, err := store.GatewayForGeneration(5)
	if err != nil || got.Endpoint != "core.example:4317" || got.Revision != 7 {
		t.Fatalf("migrated Gateway runtime = %+v, %v", got, err)
	}
}

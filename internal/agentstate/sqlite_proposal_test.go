package agentstate

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

func TestSQLiteProposalRepositoryCASDismissAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	repository := store.ProposalRepository()
	proposal := sqliteProposalFixture(t)
	if err := repository.PutProposal(proposal, 0); err != nil {
		t.Fatal(err)
	}
	if err := repository.PutProposal(proposal, 0); !errors.Is(err, discovery.ErrProposalConflict) {
		t.Fatalf("duplicate insert error = %v", err)
	}
	stale := proposal
	stale.Revision = 3
	stale.CandidateDigest = "sha256:" + repeatedProposalHex('c')
	if err := repository.PutProposal(stale, 2); !errors.Is(err, discovery.ErrProposalConflict) {
		t.Fatalf("stale insert error = %v", err)
	}
	proposals, err := NewSQLiteStore(path).ProposalRepository().ProposalSnapshot()
	if err != nil || len(proposals) != 1 || proposals[0].Revision != 1 || proposals[0].CandidateDigest != proposal.CandidateDigest {
		t.Fatalf("recovered proposals=%+v err=%v", proposals, err)
	}
	if err := repository.DismissProposal(proposal.ID, 0, time.Now().UTC()); !errors.Is(err, discovery.ErrInvalidProposal) {
		t.Fatalf("invalid dismissal error = %v", err)
	}
	if err := repository.DismissProposal(proposal.ID, 2, time.Now().UTC()); !errors.Is(err, discovery.ErrProposalConflict) {
		t.Fatalf("stale dismissal error = %v", err)
	}
	if err := repository.DismissProposal(proposal.ID, 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	proposals, err = repository.ProposalSnapshot()
	if err != nil || proposals[0].Status != discovery.ProposalDismissed || proposals[0].Revision != 1 {
		t.Fatalf("dismissed proposal=%+v err=%v", proposals, err)
	}
}

func TestSQLiteProposalRepositoryConcurrentCreateHasSingleWinner(t *testing.T) {
	repository := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db")).ProposalRepository()
	proposal := sqliteProposalFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- repository.PutProposal(proposal, 0)
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, discovery.ErrProposalConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestSQLiteNeedsInputProposalPersistsWithoutCandidateConfiguration(t *testing.T) {
	repository := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db")).ProposalRepository()
	proposal := sqliteProposalFixture(t)
	proposal.Status = discovery.ProposalNeedsInput
	proposal.CandidateDigest = ""
	proposal.CandidateConfiguration = nil
	proposal.RequiredInputs = []discovery.ProposalInput{{Name: "authentication_profile", Reason: "an approved profile is required"}}
	if err := discovery.ValidateProposal(proposal); err != nil {
		t.Fatalf("needs-input fixture is invalid: %v", err)
	}
	if err := repository.PutProposal(proposal, 0); err != nil {
		t.Fatalf("persist needs-input proposal: %v", err)
	}
	got, err := repository.ProposalSnapshot()
	if err != nil {
		t.Fatalf("read persisted needs-input proposal: %v", err)
	}
	if len(got) != 1 || got[0].Status != discovery.ProposalNeedsInput || len(got[0].CandidateConfiguration) != 0 {
		t.Fatalf("persisted needs-input proposal=%+v", got)
	}
}

func TestMonitoringProposalMigrationFromV11PreservesExistingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	want := State{DesiredGeneration: 8, ActivatedGeneration: 7, AppliedGeneration: 7, Dependencies: map[string]DependencyState{"host": {Enabled: true}}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.IntegrationRepository().Put(integrationInstance("cache", "localhost:6379"), 0); err != nil {
		t.Fatal(err)
	}
	providerSnapshot, resources := inventoryFixture(time.Now().UTC(), "container-a")
	if err := store.InventoryRepository().CommitProviderSnapshot(0, providerSnapshot, resources, true); err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE integration_proposals; DELETE FROM schema_migrations WHERE version=12`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	restarted := NewSQLiteStore(path)
	if _, err := restarted.ProposalRepository().ProposalSnapshot(); err != nil {
		t.Fatal(err)
	}
	state, err := restarted.Load()
	if err != nil || state.DesiredGeneration != want.DesiredGeneration || state.ActivatedGeneration != want.ActivatedGeneration || state.AppliedGeneration != want.AppliedGeneration || !state.Dependencies["host"].Enabled {
		t.Fatalf("M2 state after migration=%+v err=%v", state, err)
	}
	snapshot, err := restarted.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 {
		t.Fatalf("Integration state after migration=%+v err=%v", snapshot, err)
	}
	db, err = restarted.open()
	if err != nil {
		t.Fatal(err)
	}
	var inventoryCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM inventory_resources WHERE provider_resource_id='container-a'`).Scan(&inventoryCount); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	if inventoryCount != 1 {
		t.Fatalf("inventory resource count after migration=%d", inventoryCount)
	}
}

func sqliteProposalFixture(t *testing.T) discovery.Proposal {
	t.Helper()
	identity := discovery.ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"}
	id, err := discovery.StableProposalID(identity)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proposal := discovery.Proposal{
		ID: id, Identity: identity, Revision: 1, Status: discovery.ProposalPending, DefinitionVersion: "1.0.0",
		ResourceFingerprint: "sha256:" + repeatedProposalHex('a'), CandidateDigest: "sha256:" + repeatedProposalHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`), CapabilityEvidence: json.RawMessage(`{"status":"supported"}`),
		InventoryRevision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := discovery.ValidateProposal(proposal); err != nil {
		t.Fatal(err)
	}
	return proposal
}

func repeatedProposalHex(value byte) string {
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = value
	}
	return string(bytes)
}

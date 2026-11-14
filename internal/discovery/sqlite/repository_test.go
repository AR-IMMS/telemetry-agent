package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverysqlite "github.com/ar-imms/telemetry-agent/internal/discovery/sqlite"
)

func TestInventoryRepositoryKeepsFailureEvidenceAndWriteCAS(t *testing.T) {
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := discoverysqlite.NewInventoryRepository(store.OpenDatabase, &sync.Mutex{})
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	snapshot := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{{ProviderResourceID: "container-a", Kind: discovery.ResourceContainer}},
	}
	resources := []discovery.Resource{{
		Key:  discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Kind: discovery.ResourceContainer, Presence: discovery.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
	}}
	if err := repository.CommitProviderSnapshot(0, snapshot, resources, true); err != nil {
		t.Fatalf("commit complete provider snapshot: %v", err)
	}
	before, err := repository.Snapshot()
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if before.Revision != 1 || before.WriteRevision != 1 || len(before.Resources) != 1 {
		t.Fatalf("initial snapshot = %+v", before)
	}
	if err := repository.RecordProviderFailure(discovery.ProviderStatus{
		ProviderID: "docker", ProviderInstanceID: "local", Enabled: true,
		LastAttemptAt: at.Add(time.Minute), LastErrorCode: "daemon_unavailable",
	}); err != nil {
		t.Fatalf("record provider failure: %v", err)
	}
	if err := repository.CommitProviderSnapshot(before.WriteRevision, snapshot, resources, false); !errors.Is(err, discovery.ErrSnapshotConflict) {
		t.Fatalf("commit with stale write revision = %v, want snapshot conflict", err)
	}
	after, err := repository.Snapshot()
	if err != nil {
		t.Fatalf("read inventory after provider failure: %v", err)
	}
	if after.Revision != before.Revision || after.WriteRevision != before.WriteRevision+1 || len(after.Resources) != 1 ||
		after.Providers[0].LastErrorCode != "daemon_unavailable" || !after.Providers[0].LastSuccessAt.Equal(at) {
		t.Fatalf("failure changed material evidence or lost provider status: %+v", after)
	}
}

func TestInventoryRepositoryReadsBoundedProviderStatusesWithoutResources(t *testing.T) {
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := discoverysqlite.NewInventoryRepository(store.OpenDatabase, &sync.Mutex{})
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	for index := 0; index < 3; index++ {
		instance := discovery.ProviderInstanceID(fmt.Sprintf("instance-%d", index))
		resourceID := fmt.Sprintf("container-%d", index)
		providerSnapshot := discovery.ProviderSnapshot{
			ProviderID: "docker", ProviderInstanceID: instance, Complete: true, ObservedAt: at,
			Resources: []discovery.ResourceObservation{{ProviderResourceID: resourceID, Kind: discovery.ResourceContainer}},
		}
		resource := discovery.Resource{
			Key:  discovery.ResourceKey{Provider: "docker", ProviderInstance: instance, ProviderResourceID: resourceID},
			Kind: discovery.ResourceContainer, Presence: discovery.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
		}
		if err := repository.CommitProviderSnapshot(uint64(index), providerSnapshot, []discovery.Resource{resource}, true); err != nil {
			t.Fatalf("commit provider snapshot %d: %v", index, err)
		}
	}

	reader, ok := repository.(discovery.ProviderStatusReader)
	if !ok {
		t.Fatal("inventory repository does not expose bounded provider status reader")
	}
	statuses, truncated, err := reader.ReadProviderStatuses(context.Background(), 2)
	if err != nil {
		t.Fatalf("read limited provider statuses: %v", err)
	}
	if len(statuses) != 2 || !truncated {
		t.Fatalf("limited statuses=(%d, truncated=%t), want (2, true)", len(statuses), truncated)
	}
	statuses, truncated, err = reader.ReadProviderStatuses(context.Background(), discovery.MaxProviderStatusReadLimit)
	if err != nil {
		t.Fatalf("read provider statuses: %v", err)
	}
	if len(statuses) != 3 || truncated {
		t.Fatalf("all statuses=(%d, truncated=%t), want (3, false)", len(statuses), truncated)
	}
	for _, limit := range []int{0, discovery.MaxProviderStatusReadLimit + 1} {
		if _, _, err := reader.ReadProviderStatuses(context.Background(), limit); err == nil {
			t.Fatalf("ReadProviderStatuses(%d) error=nil, want invalid limit error", limit)
		}
	}
}

func TestInventoryRepositoryProviderStatusReadHonorsCancellation(t *testing.T) {
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := discoverysqlite.NewInventoryRepository(store.OpenDatabase, &sync.Mutex{})
	reader := repository.(discovery.ProviderStatusReader)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := reader.ReadProviderStatuses(ctx, discovery.MaxProviderStatusReadLimit)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadProviderStatuses error=%v, want caller cancellation", err)
	}
}

func TestProposalRepositoryPersistsCASDismissalAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := agentstate.NewSQLiteStore(path)
	locker := &sync.Mutex{}
	repository := discoverysqlite.NewProposalRepository(store.OpenDatabase, locker)
	proposal := proposalFixture(t)
	if err := repository.PutProposal(proposal, 0); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	if err := repository.PutProposal(proposal, 0); !errors.Is(err, discovery.ErrProposalConflict) {
		t.Fatalf("duplicate proposal insert = %v, want proposal conflict", err)
	}
	if err := repository.DismissProposal(proposal.ID, 2, time.Now().UTC()); !errors.Is(err, discovery.ErrProposalConflict) {
		t.Fatalf("dismiss with stale revision = %v, want proposal conflict", err)
	}
	if err := repository.DismissProposal(proposal.ID, 1, time.Now().UTC()); err != nil {
		t.Fatalf("dismiss current proposal: %v", err)
	}
	proposals, err := discoverysqlite.NewProposalRepository(store.OpenDatabase, locker).ProposalSnapshot()
	if err != nil {
		t.Fatalf("read proposal after reopening repository: %v", err)
	}
	if len(proposals) != 1 || proposals[0].Status != discovery.ProposalDismissed || proposals[0].Revision != proposal.Revision {
		t.Fatalf("persisted proposals = %+v", proposals)
	}
}

func proposalFixture(t *testing.T) discovery.Proposal {
	t.Helper()
	identity := discovery.ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"}
	id, err := discovery.StableProposalID(identity)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proposal := discovery.Proposal{
		ID: id, Identity: identity, Revision: 1, Status: discovery.ProposalPending, DefinitionVersion: "1.0.0",
		ResourceFingerprint: "sha256:" + repeatedHex('a'), CandidateDigest: "sha256:" + repeatedHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`),
		CapabilityEvidence:     json.RawMessage(`{"status":"supported"}`), InventoryRevision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := discovery.ValidateProposal(proposal); err != nil {
		t.Fatal(err)
	}
	return proposal
}

func repeatedHex(value byte) string {
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = value
	}
	return string(bytes)
}

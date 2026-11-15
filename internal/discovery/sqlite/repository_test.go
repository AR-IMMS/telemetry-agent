package sqlite_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverysqlite "github.com/ar-imms/telemetry-agent/internal/discovery/sqlite"
)

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

package sqlite_test

import (
	"encoding/json"
	"errors"
	proposals "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestProposalRepositoryPersistsCASDismissalAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := sqlitefixture.NewSQLiteStore(path)
	locker := &sync.Mutex{}
	repository := configurationsqlite.NewProposalRepository(store.OpenDatabase, locker)
	proposal := proposalFixture(t)
	if err := repository.PutProposal(proposal, 0); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	if err := repository.PutProposal(proposal, 0); !errors.Is(err, proposals.ErrProposalConflict) {
		t.Fatalf("duplicate proposal insert = %v, want proposal conflict", err)
	}
	if err := repository.DismissProposal(proposal.ID, 2, time.Now().UTC()); !errors.Is(err, proposals.ErrProposalConflict) {
		t.Fatalf("dismiss with stale revision = %v, want proposal conflict", err)
	}
	if err := repository.DismissProposal(proposal.ID, 1, time.Now().UTC()); err != nil {
		t.Fatalf("dismiss current proposal: %v", err)
	}
	proposalList, err := configurationsqlite.NewProposalRepository(store.OpenDatabase, locker).ProposalSnapshot()
	if err != nil {
		t.Fatalf("read proposal after reopening repository: %v", err)
	}
	if len(proposalList) != 1 || proposalList[0].Status != proposals.ProposalDismissed || proposalList[0].Revision != proposal.Revision {
		t.Fatalf("persisted proposals = %+v", proposalList)
	}
}

func proposalFixture(t *testing.T) proposals.Proposal {
	t.Helper()
	identity := proposals.ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"}
	id, err := proposals.StableProposalID(identity)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proposal := proposals.Proposal{
		ID: id, Identity: identity, Revision: 1, Status: proposals.ProposalPending, DefinitionVersion: "1.0.0",
		ResourceFingerprint: "sha256:" + proposalRepeatedHex('a'), CandidateDigest: "sha256:" + proposalRepeatedHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`),
		CapabilityEvidence:     json.RawMessage(`{"status":"supported"}`), InventoryRevision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := proposals.ValidateProposal(proposal); err != nil {
		t.Fatal(err)
	}
	return proposal
}

func proposalRepeatedHex(value byte) string {
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = value
	}
	return string(bytes)
}

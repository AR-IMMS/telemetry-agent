package agentstate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestSQLiteApprovalCommitsProposalAndDesiredStateAtomically(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	proposal := sqliteProposalFixture(t)
	if err := store.ProposalRepository().PutProposal(proposal, 0); err != nil {
		t.Fatal(err)
	}
	commit := approvalFixture(proposal, 0)
	instance, revision, err := store.ApprovalRepository().CommitApproval(commit)
	if err != nil || instance.Revision != 1 || revision != 1 {
		t.Fatalf("approval instance=%+v desired revision=%d err=%v", instance, revision, err)
	}
	proposals, err := store.ProposalRepository().ProposalSnapshot()
	if err != nil || proposals[0].Status != discovery.ProposalApproved || proposals[0].ApprovedInstanceID != instance.ID || proposals[0].ApprovedCandidateDigest != proposal.CandidateDigest {
		t.Fatalf("proposal after approval=%+v err=%v", proposals, err)
	}
	commit.Instance = instance
	if got, desired, err := store.ApprovalRepository().CommitApproval(commit); err != nil || got.Revision != 1 || desired != 1 {
		t.Fatalf("idempotent approval=%+v desired=%d err=%v", got, desired, err)
	}
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 {
		t.Fatalf("desired snapshot=%+v err=%v", snapshot, err)
	}
}

func TestSQLiteApprovalRollsBackEveryWriteWhenTerminalProposalUpdateFails(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	proposal := sqliteProposalFixture(t)
	if err := store.ProposalRepository().PutProposal(proposal, 0); err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_proposal_approval BEFORE UPDATE ON integration_proposals BEGIN SELECT RAISE(ABORT,'injected approval failure'); END`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	if _, _, err := store.ApprovalRepository().CommitApproval(approvalFixture(proposal, 0)); err == nil {
		t.Fatal("approval succeeded despite injected proposal write failure")
	}
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 0 || len(snapshot.Instances) != 0 {
		t.Fatalf("partial desired mutation survived rollback: %+v err=%v", snapshot, err)
	}
	proposals, err := store.ProposalRepository().ProposalSnapshot()
	if err != nil || proposals[0].Status != discovery.ProposalPending {
		t.Fatalf("proposal changed despite rollback: %+v err=%v", proposals, err)
	}
}

func TestSQLiteApprovalRejectsStaleDesiredAndInventorySnapshots(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*configuration.ApprovalCommit)
		want   error
	}{
		{name: "desired revision", mutate: func(commit *configuration.ApprovalCommit) { commit.ExpectedDesiredRevision = 1 }, want: configuration.ErrRevisionConflict},
		{name: "inventory write revision", mutate: func(commit *configuration.ApprovalCommit) { commit.InventoryWriteRevision = 1 }, want: configuration.ErrApprovalStale},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			proposal := sqliteProposalFixture(t)
			if err := store.ProposalRepository().PutProposal(proposal, 0); err != nil {
				t.Fatal(err)
			}
			commit := approvalFixture(proposal, 0)
			test.mutate(&commit)
			if _, _, err := store.ApprovalRepository().CommitApproval(commit); !errors.Is(err, test.want) {
				t.Fatalf("stale admission error=%v, want %v", err, test.want)
			}
		})
	}
}

func TestSQLiteApprovalRejectsDuplicateResourceBinding(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	first := sqliteProposalFixture(t)
	if err := store.ProposalRepository().PutProposal(first, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ApprovalRepository().CommitApproval(approvalFixture(first, 0)); err != nil {
		t.Fatal(err)
	}
	second := sqliteProposalFixture(t)
	second.Identity.IntegrationID = "postgresql"
	second.ID, _ = discovery.StableProposalID(second.Identity)
	second.CandidateDigest = "sha256:" + repeatedProposalHex('d')
	second.ResourceFingerprint = "sha256:" + repeatedProposalHex('e')
	second.Revision = 1
	second.Status = discovery.ProposalPending
	if err := store.ProposalRepository().PutProposal(second, 0); err != nil {
		t.Fatal(err)
	}
	commit := approvalFixture(second, 1)
	if _, _, err := store.ApprovalRepository().CommitApproval(commit); !errors.Is(err, configuration.ErrApprovalBindingConflict) {
		t.Fatalf("duplicate binding error=%v", err)
	}
}

func TestSQLiteApprovalRejectsEquivalentManualIntegration(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	proposal := sqliteProposalFixture(t)
	if err := store.ProposalRepository().PutProposal(proposal, 0); err != nil {
		t.Fatal(err)
	}
	manual := integration.IntegrationInstance{ID: "manual-cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}}
	if _, _, err := store.IntegrationRepository().Put(manual, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ApprovalRepository().CommitApproval(approvalFixture(proposal, 1)); !errors.Is(err, configuration.ErrApprovalDuplicate) {
		t.Fatalf("duplicate manual configuration error=%v", err)
	}
}

func TestSQLiteApprovalConcurrentRepeatAdvancesDesiredSetOnce(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	proposal := sqliteProposalFixture(t)
	if err := store.ProposalRepository().PutProposal(proposal, 0); err != nil {
		t.Fatal(err)
	}
	commit := approvalFixture(proposal, 0)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, _, err := store.ApprovalRepository().CommitApproval(commit)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent idempotent approval error=%v", err)
		}
	}
	snapshot, err := NewSQLiteStore(store.path).IntegrationRepository().Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Instances) != 1 || snapshot.Instances[0].Revision != 1 {
		t.Fatalf("concurrent approval desired snapshot=%+v err=%v", snapshot, err)
	}
}

func approvalFixture(proposal discovery.Proposal, expectedDesired uint64) configuration.ApprovalCommit {
	return configuration.ApprovalCommit{
		ProposalID: proposal.ID, ProposalRevision: proposal.Revision, ExpectedDesiredRevision: expectedDesired,
		ExpectedCandidateDigest: proposal.CandidateDigest, VerifiedCandidateDigest: proposal.CandidateDigest,
		InventoryWriteRevision: 0,
		Instance: integration.IntegrationInstance{
			ID: discoveredTestInstanceID(proposal.ID), DefinitionID: proposal.Identity.IntegrationID, DefinitionVersion: proposal.DefinitionVersion,
			Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Source: integration.SourceDiscovery,
			Binding: &integration.ResourceBinding{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID}, Enabled: true,
		},
	}
}

func discoveredTestInstanceID(proposalID string) string {
	digest := sha256.Sum256([]byte(proposalID))
	return "discovered-" + hex.EncodeToString(digest[:12])
}

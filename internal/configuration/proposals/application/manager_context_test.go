package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingProposalRepository struct {
	started chan struct{}
}

func (r blockingProposalRepository) ProposalSnapshot() ([]Proposal, error) { return nil, nil }
func (r blockingProposalRepository) ProposalSnapshotContext(ctx context.Context) ([]Proposal, error) {
	close(r.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingProposalRepository) PutProposal(Proposal, uint64) error { return nil }
func (blockingProposalRepository) PutProposalContext(context.Context, Proposal, uint64) error {
	return nil
}
func (blockingProposalRepository) DismissProposal(string, uint64, time.Time) error { return nil }
func (blockingProposalRepository) DismissProposalContext(context.Context, string, uint64, time.Time) error {
	return nil
}

func TestProposalManagerRefreshPropagatesCancellationToRepositoryRead(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	manager := NewProposalManager(blockingProposalRepository{started: started})
	done := make(chan error, 1)
	go func() { done <- manager.RefreshContext(ctx, nil, time.Now().UTC()) }()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("proposal refresh did not reach its repository read")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Refresh() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("proposal refresh did not stop after its repository read was canceled")
	}
}

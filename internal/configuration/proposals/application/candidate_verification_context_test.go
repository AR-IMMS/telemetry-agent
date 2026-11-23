package application

import (
	"context"
	"errors"
	"testing"
	"time"

	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
)

func TestCandidateVerificationStopsWhenInventoryReadIsCanceled(t *testing.T) {
	started := make(chan struct{})
	service := NewCandidateVerificationService(
		blockingInventorySnapshotReader{started: started}, emptyProposalMatcher{}, definitionapp.NewResolver(definitionapp.Registry{}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.Verify(ctx, Proposal{}, nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("candidate verification did not start the inventory read")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Verify() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("candidate verification did not stop when inventory read was canceled")
	}
}

package application

import (
	"context"
	"errors"
	"testing"
	"time"

	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	matchingapp "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
	monitoringtemplatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	monitoringtemplatesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
)

type blockingInventorySnapshotReader struct{ started chan struct{} }

func (r blockingInventorySnapshotReader) SnapshotContext(ctx context.Context) (inventorydomain.InventorySnapshot, error) {
	close(r.started)
	<-ctx.Done()
	return inventorydomain.InventorySnapshot{}, ctx.Err()
}

type emptyTemplateSnapshotReader struct{}

func (emptyTemplateSnapshotReader) SnapshotContext(context.Context) (templatesapp.Snapshot, error) {
	return templatesapp.Snapshot{}, nil
}

type emptyProposalMatcher struct{}

func (emptyProposalMatcher) Match(snapshot matchingapp.MatchSnapshot) (matchingapp.MatchReport, error) {
	return matchingapp.MatchReport{InventoryRevision: snapshot.InventoryRevision, EvaluatedAt: snapshot.AsOf}, nil
}

type emptyProposalTemplateEvaluator struct{}

func (emptyProposalTemplateEvaluator) MatchTemplates(snapshot matchingapp.MatchSnapshot, _ []monitoringtemplatesdomain.TemplateInput) (monitoringtemplatesapp.TemplateMatchReport, error) {
	return monitoringtemplatesapp.TemplateMatchReport{InventoryRevision: snapshot.InventoryRevision, EvaluatedAt: snapshot.AsOf}, nil
}

func TestProposalRefreshServiceStopsWhenInventoryReadIsCanceled(t *testing.T) {
	started := make(chan struct{})
	service := NewProposalRefreshService(
		blockingInventorySnapshotReader{started: started}, emptyTemplateSnapshotReader{},
		NewProposalManager(nil), emptyProposalMatcher{}, emptyProposalTemplateEvaluator{},
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Refresh(ctx) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("proposal refresh did not start the inventory read")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Refresh() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("proposal refresh did not stop when inventory read was canceled")
	}
}

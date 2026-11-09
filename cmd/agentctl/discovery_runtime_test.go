package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

type runtimeInventoryRepository struct {
	revision atomic.Uint64
	writes   atomic.Uint64
}

func (r *runtimeInventoryRepository) Snapshot() (discovery.InventorySnapshot, error) {
	return discovery.InventorySnapshot{Revision: r.revision.Load(), WriteRevision: r.writes.Load()}, nil
}

func (r *runtimeInventoryRepository) CommitProviderSnapshot(expectedWriteRevision uint64, _ discovery.ProviderSnapshot, _ []discovery.Resource, changed bool) error {
	if r.writes.Load() != expectedWriteRevision {
		return discovery.ErrSnapshotConflict
	}
	if changed {
		r.revision.Add(1)
	}
	r.writes.Add(1)
	return nil
}

func (r *runtimeInventoryRepository) RecordProviderFailure(discovery.ProviderStatus) error {
	return nil
}

type runtimeDiscoveryProvider struct {
	calls atomic.Int32
	seen  chan struct{}
}

func (p *runtimeDiscoveryProvider) ID() discovery.ProviderID                 { return "docker" }
func (p *runtimeDiscoveryProvider) InstanceID() discovery.ProviderInstanceID { return "local" }
func (p *runtimeDiscoveryProvider) Discover(context.Context) (discovery.ProviderSnapshot, error) {
	p.calls.Add(1)
	select {
	case p.seen <- struct{}{}:
	default:
	}
	return discovery.ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, Resources: []discovery.ResourceObservation{}}, nil
}

func TestRuntimeControlLoopsRunsDiscoveryAndStopsWithReconciliation(t *testing.T) {
	provider := &runtimeDiscoveryProvider{seen: make(chan struct{}, 1)}
	coordinator, err := discovery.NewCoordinator(&runtimeInventoryRepository{}, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("reconciliation stopped")
	reconcile := func(ctx context.Context) error {
		select {
		case <-provider.seen:
			return want
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			return errors.New("discovery did not run")
		}
	}
	if err := runRuntimeControlLoops(context.Background(), reconcile, coordinator); !errors.Is(err, want) {
		t.Fatalf("runtime error=%v, want %v", err, want)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls=%d, want one initial snapshot", provider.calls.Load())
	}
}

func TestRuntimeControlLoopsWithoutDiscoveryPreservesReconciliation(t *testing.T) {
	want := errors.New("reconciliation failure")
	if err := runRuntimeControlLoops(context.Background(), func(context.Context) error { return want }, nil); !errors.Is(err, want) {
		t.Fatalf("runtime error=%v, want %v", err, want)
	}
}

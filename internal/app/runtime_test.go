package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation"
)

type runtimeReconciler struct {
	run func(context.Context, time.Duration) error
}

func (r runtimeReconciler) Reconcile(context.Context) (reconciliation.Result, error) {
	return reconciliation.Result{}, nil
}

func (r runtimeReconciler) Run(ctx context.Context, interval time.Duration) error {
	return r.run(ctx, interval)
}

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
	err   error
}

func (p *runtimeDiscoveryProvider) ID() discovery.ProviderID                 { return "docker" }
func (p *runtimeDiscoveryProvider) InstanceID() discovery.ProviderInstanceID { return "local" }
func (p *runtimeDiscoveryProvider) Discover(context.Context) (discovery.ProviderSnapshot, error) {
	p.calls.Add(1)
	select {
	case p.seen <- struct{}{}:
	default:
	}
	if p.err != nil {
		return discovery.ProviderSnapshot{}, p.err
	}
	return discovery.ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true}, nil
}

func TestRunControlLoopsRunsDiscoveryAndStopsWithReconciliation(t *testing.T) {
	provider := &runtimeDiscoveryProvider{seen: make(chan struct{}, 1)}
	coordinator, err := discovery.NewCoordinator(&runtimeInventoryRepository{}, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("reconciliation stopped")
	var refreshes atomic.Int32
	components := RuntimeComponents{
		Inventory: coordinator,
		Reconciler: runtimeReconciler{run: func(ctx context.Context, _ time.Duration) error {
			select {
			case <-provider.seen:
				return want
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
				return errors.New("discovery did not run")
			}
		}},
		RefreshProposals: func(context.Context) error { refreshes.Add(1); return nil },
	}
	if err := runControlLoops(context.Background(), components); !errors.Is(err, want) {
		t.Fatalf("runtime error=%v, want %v", err, want)
	}
	if provider.calls.Load() != 1 || refreshes.Load() != 1 {
		t.Fatalf("provider calls=%d proposal refreshes=%d, want 1 each", provider.calls.Load(), refreshes.Load())
	}
}

func TestRunControlLoopsWithoutDiscoveryPreservesReconciliation(t *testing.T) {
	want := errors.New("reconciliation failure")
	var refreshes atomic.Int32
	components := RuntimeComponents{
		Reconciler:       runtimeReconciler{run: func(context.Context, time.Duration) error { return want }},
		RefreshProposals: func(context.Context) error { refreshes.Add(1); return nil },
	}
	if err := runControlLoops(context.Background(), components); !errors.Is(err, want) {
		t.Fatalf("runtime error=%v, want %v", err, want)
	}
	if refreshes.Load() != 1 {
		t.Fatalf("proposal refresh calls=%d, want one startup refresh", refreshes.Load())
	}
}

func TestRunControlLoopsSkipsTemplateProjectionAfterProviderFailure(t *testing.T) {
	provider := &runtimeDiscoveryProvider{seen: make(chan struct{}, 1), err: errors.New("Docker unavailable")}
	coordinator, err := discovery.NewCoordinator(&runtimeInventoryRepository{}, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("reconciliation stopped")
	var proposals, templates atomic.Int32
	components := RuntimeComponents{
		Inventory: coordinator,
		Reconciler: runtimeReconciler{run: func(ctx context.Context, _ time.Duration) error {
			select {
			case <-provider.seen:
				return want
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
				return errors.New("discovery did not run")
			}
		}},
		RefreshProposals: func(context.Context) error { proposals.Add(1); return nil },
		RefreshTemplates: func(context.Context) error { templates.Add(1); return nil },
	}
	if err := runControlLoops(context.Background(), components); !errors.Is(err, want) {
		t.Fatalf("runtime error=%v, want %v", err, want)
	}
	if proposals.Load() != 1 || templates.Load() != 0 {
		t.Fatalf("proposal refreshes=%d template refreshes=%d, want 1 and 0", proposals.Load(), templates.Load())
	}
}

func TestRunRuntimeRequiresBuilder(t *testing.T) {
	if err := RunRuntime(context.Background(), RuntimeOptions{}); err == nil {
		t.Fatal("RunRuntime() without options unexpectedly succeeded")
	}
}

package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

type testRepository struct {
	snapshot    domain.InventorySnapshot
	failure     domain.ProviderStatus
	commitCalls int
}

func (r *testRepository) Snapshot() (domain.InventorySnapshot, error) { return r.snapshot, nil }

func (r *testRepository) CommitProviderSnapshot(_ uint64, _ domain.ProviderSnapshot, resources []domain.Resource, _ bool) error {
	r.commitCalls++
	r.snapshot.Resources = resources
	return nil
}

func (r *testRepository) RecordProviderFailure(status domain.ProviderStatus) error {
	r.failure = status
	return nil
}

type testProvider struct{ err error }

func (testProvider) ID() domain.ProviderID                 { return "docker" }
func (testProvider) InstanceID() domain.ProviderInstanceID { return "local" }
func (p testProvider) Discover(context.Context) (domain.ProviderSnapshot, error) {
	return domain.ProviderSnapshot{}, p.err
}

func TestDiscoveryFailureRecordsEvidenceWithoutCommittingAbsence(t *testing.T) {
	existing := domain.Resource{
		Key:  domain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-1"},
		Kind: domain.ResourceContainer, Presence: domain.PresencePresent,
	}
	repository := &testRepository{snapshot: domain.InventorySnapshot{Resources: []domain.Resource{existing}}}
	coordinator, err := NewCoordinator(repository, []DiscoveryProvider{testProvider{err: errors.New("daemon unavailable")}}, domain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	result, err := coordinator.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatalf("DiscoverOnce() error = %v", err)
	}
	if result.Succeeded() || len(result.ProviderResults) != 1 || result.ProviderResults[0].ErrorCode != "provider_error" {
		t.Fatalf("DiscoverOnce() result = %#v, want one provider_error", result)
	}
	if repository.commitCalls != 0 {
		t.Fatalf("CommitProviderSnapshot() calls = %d, want 0", repository.commitCalls)
	}
	if len(repository.snapshot.Resources) != 1 || repository.snapshot.Resources[0].Key.ProviderResourceID != "container-1" || repository.snapshot.Resources[0].Presence != domain.PresencePresent {
		t.Fatalf("inventory after provider failure = %#v, want existing resource preserved", repository.snapshot.Resources)
	}
	if repository.failure.LastErrorCode != "provider_error" {
		t.Fatalf("failure diagnostic = %#v, want provider_error", repository.failure)
	}
}

func TestCycleResultSucceededUsesProviderOutcomes(t *testing.T) {
	if !(CycleResult{}).Succeeded() {
		t.Fatal("empty configured provider set should retain successful-cycle semantics")
	}
	if !(CycleResult{ProviderResults: []ProviderResult{{ProviderID: "docker", ProviderInstanceID: "local"}}}).Succeeded() {
		t.Fatal("successful provider cycle reported failure")
	}
	if (CycleResult{ProviderResults: []ProviderResult{
		{ProviderID: "docker", ProviderInstanceID: "local"},
		{ProviderID: "docker", ProviderInstanceID: "remote", ErrorCode: "daemon_unavailable"},
	}}).Succeeded() {
		t.Fatal("cycle with a failed provider reported success")
	}
}

type recoveryRepository struct{ snapshot domain.InventorySnapshot }

func (r *recoveryRepository) Snapshot() (domain.InventorySnapshot, error) {
	return r.snapshot, nil
}

func (r *recoveryRepository) CommitProviderSnapshot(expected uint64, snapshot domain.ProviderSnapshot, resources []domain.Resource, changed bool) error {
	if r.snapshot.WriteRevision != expected {
		return ErrSnapshotConflict
	}
	retained := make([]domain.Resource, 0, len(r.snapshot.Resources)+len(resources))
	for _, current := range r.snapshot.Resources {
		if current.Key.Provider != snapshot.ProviderID || current.Key.ProviderInstance != snapshot.ProviderInstanceID {
			retained = append(retained, current)
		}
	}
	r.snapshot.Resources = append(retained, resources...)
	if changed {
		r.snapshot.Revision++
	}
	r.snapshot.WriteRevision++
	r.snapshot.Providers = []domain.ProviderStatus{{
		ProviderID: snapshot.ProviderID, ProviderInstanceID: snapshot.ProviderInstanceID,
		Enabled: true, LastAttemptAt: snapshot.ObservedAt, LastSuccessAt: snapshot.ObservedAt,
		ResourceCount: len(snapshot.Resources),
	}}
	return nil
}

func (r *recoveryRepository) RecordProviderFailure(status domain.ProviderStatus) error {
	r.snapshot.Providers = []domain.ProviderStatus{status}
	return nil
}

type recoveryProvider struct {
	err      error
	snapshot domain.ProviderSnapshot
}

func (recoveryProvider) ID() domain.ProviderID                 { return "docker" }
func (recoveryProvider) InstanceID() domain.ProviderInstanceID { return "local" }
func (p recoveryProvider) Discover(context.Context) (domain.ProviderSnapshot, error) {
	return p.snapshot, p.err
}

func TestProviderFailurePreservesInventoryAndRecoveryRefreshesIt(t *testing.T) {
	at := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	existing := domain.Resource{
		Key:  domain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-1"},
		Kind: domain.ResourceContainer, Presence: domain.PresencePresent, LastSeenAt: at,
	}
	repository := &recoveryRepository{snapshot: domain.InventorySnapshot{Resources: []domain.Resource{existing}}}
	failed, err := NewCoordinator(repository, []DiscoveryProvider{recoveryProvider{err: errors.New("permission denied")}}, domain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := failed.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded() || repository.snapshot.Resources[0].Presence != domain.PresencePresent || repository.snapshot.Revision != 0 {
		t.Fatalf("failed observation changed inventory: result=%+v snapshot=%+v", result, repository.snapshot)
	}
	if repository.snapshot.Providers[0].LastErrorCode != "permission_denied" {
		t.Fatalf("provider diagnostic=%+v", repository.snapshot.Providers)
	}

	snapshot := domain.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at.Add(time.Minute),
		Resources: []domain.ResourceObservation{{ProviderResourceID: "container-1", Kind: domain.ResourceContainer, LifecycleState: "running", Endpoints: []domain.Endpoint{{Address: "127.0.0.1", Port: 16379, ContainerPort: 6379, Protocol: "tcp", Scope: domain.EndpointHost, Provenance: "published_port"}}}},
	}
	recovered, err := NewCoordinator(repository, []DiscoveryProvider{recoveryProvider{snapshot: snapshot}}, domain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err = recovered.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded() || repository.snapshot.Providers[0].LastErrorCode != "" {
		t.Fatalf("recovery retained failure: result=%+v providers=%+v", result, repository.snapshot.Providers)
	}
	resource := repository.snapshot.Resources[0]
	if resource.Presence != domain.PresencePresent || len(resource.Endpoints) != 1 || resource.Endpoints[0].Port != 16379 {
		t.Fatalf("recovery did not refresh resource evidence: %+v", resource)
	}
}

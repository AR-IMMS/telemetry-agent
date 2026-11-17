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

package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeSnapshotRejectsIncompleteEvidence(t *testing.T) {
	_, err := NormalizeSnapshot(ProviderSnapshot{
		ProviderID:         "docker",
		ProviderInstanceID: "local",
		ObservedAt:         time.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC),
		Complete:           false,
	})
	if !errors.Is(err, ErrIncompleteSnapshot) {
		t.Fatalf("NormalizeSnapshot() error = %v, want ErrIncompleteSnapshot", err)
	}
}

func TestReconcileIncompleteSnapshotPreservesCurrentResource(t *testing.T) {
	existing := Resource{
		Key:  ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-1"},
		Kind: ResourceContainer, Presence: PresencePresent,
	}
	resources, changed, err := ReconcileProviderSnapshot([]Resource{existing}, ProviderSnapshot{
		ProviderID:         "docker",
		ProviderInstanceID: "local",
		ObservedAt:         time.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC),
		Complete:           false,
	}, DefaultPolicy())
	if !errors.Is(err, ErrIncompleteSnapshot) {
		t.Fatalf("ReconcileProviderSnapshot() error = %v, want ErrIncompleteSnapshot", err)
	}
	if changed {
		t.Fatal("ReconcileProviderSnapshot() changed = true for incomplete evidence")
	}
	if resources != nil {
		t.Fatalf("ReconcileProviderSnapshot() resources = %#v, want nil", resources)
	}
}

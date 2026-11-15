package inventory

import (
	"errors"
	"testing"
	"time"
)

func TestIncompleteProviderSnapshotIsRejected(t *testing.T) {
	_, err := NormalizeSnapshot(ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", ObservedAt: time.Now().UTC(), Complete: false,
	})
	if !errors.Is(err, ErrIncompleteSnapshot) {
		t.Fatalf("NormalizeSnapshot() error = %v, want ErrIncompleteSnapshot", err)
	}
}

package discovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testSnapshot(at time.Time, resources ...ResourceObservation) ProviderSnapshot {
	return ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at, Resources: resources}
}

func observation(id, state string) ResourceObservation {
	return ResourceObservation{ProviderResourceID: id, Kind: ResourceContainer, DisplayName: id, LifecycleState: state, Attributes: map[string]string{"safe": "value"}, Endpoints: []Endpoint{{Address: "10.0.0.2", Port: 6379, Protocol: "tcp", Scope: EndpointContainer, Provenance: "container_network"}}}
}

func TestNormalizeSnapshotRequiresCompleteAndDeduplicatesIdenticalRecords(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := NormalizeSnapshot(ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", ObservedAt: at}); !errors.Is(err, ErrIncompleteSnapshot) {
		t.Fatalf("incomplete error=%v", err)
	}
	item := observation("id", "running")
	got, err := NormalizeSnapshot(testSnapshot(at, item, item))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Resources) != 1 {
		t.Fatalf("resources=%d, want deduplicated", len(got.Resources))
	}
	item.Attributes["safe"] = "mutated"
	item.Endpoints[0].Address = "changed"
	if got.Resources[0].Attributes["safe"] != "value" || got.Resources[0].Endpoints[0].Address != "10.0.0.2" {
		t.Fatalf("normalization did not deep-copy: %+v", got.Resources[0])
	}
}

func TestNormalizeSnapshotRejectsConflictingDuplicatesAndMalformedInput(t *testing.T) {
	at := time.Now().UTC()
	a, b := observation("same", "running"), observation("same", "exited")
	if _, err := NormalizeSnapshot(testSnapshot(at, a, b)); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	for _, item := range []ResourceObservation{
		{ProviderResourceID: "bad\nid", Kind: ResourceContainer},
		{ProviderResourceID: "id", Kind: "unknown"},
		{ProviderResourceID: "id", Kind: ResourceContainer, Attributes: map[string]string{"password": "unsafe"}},
		{ProviderResourceID: "id/ambiguous", Kind: ResourceContainer},
		{ProviderResourceID: "id", Kind: ResourceContainer, Endpoints: []Endpoint{{Port: 0, Scope: EndpointHost, Provenance: "published_port"}}},
	} {
		if _, err := NormalizeSnapshot(testSnapshot(at, item)); err == nil {
			t.Fatalf("malformed observation accepted: %+v", item)
		}
	}
}

func TestInventoryMaterialRevisionIgnoresIdenticalPollAndTracksPresenceChanges(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	policy := Policy{MissingAfterSnapshots: 2, StaleAfter: time.Minute}
	first, changed, err := ReconcileProviderSnapshot(nil, testSnapshot(at, observation("id", "running")), policy)
	if err != nil || !changed {
		t.Fatalf("initial reconciliation changed=%v err=%v", changed, err)
	}
	next, changed, err := ReconcileProviderSnapshot(first, testSnapshot(at.Add(30*time.Second), observation("id", "running")), policy)
	if err != nil || changed {
		t.Fatalf("identical poll changed=%v err=%v", changed, err)
	}
	if !next[0].LastSeenAt.Equal(at.Add(30 * time.Second)) {
		t.Fatalf("last seen=%s", next[0].LastSeenAt)
	}
	next, changed, err = ReconcileProviderSnapshot(next, testSnapshot(at.Add(time.Minute)), policy)
	if err != nil || changed || next[0].Presence != PresencePresent {
		t.Fatalf("first miss changed=%v presence=%s err=%v", changed, next[0].Presence, err)
	}
	next, changed, err = ReconcileProviderSnapshot(next, testSnapshot(at.Add(90*time.Second)), policy)
	if err != nil || !changed || next[0].Presence != PresenceMissing {
		t.Fatalf("second miss changed=%v presence=%s err=%v", changed, next[0].Presence, err)
	}
	next, changed, err = ReconcileProviderSnapshot(next, testSnapshot(at.Add(2*time.Minute), observation("id", "running")), policy)
	if err != nil || !changed || next[0].Presence != PresencePresent || next[0].MissingSnapshots != 0 || next[0].MissingSince != nil {
		t.Fatalf("reappearance changed=%v resource=%+v err=%v", changed, next[0], err)
	}
}

func TestFreshnessIsIndependentFromPresence(t *testing.T) {
	now := time.Now()
	resource := Resource{Presence: PresencePresent, LastSeenAt: now.Add(-3 * time.Minute)}
	if got := resource.FreshnessAt(now, 2*time.Minute); got != "stale" {
		t.Fatalf("freshness=%q", got)
	}
	if resource.Presence != PresencePresent {
		t.Fatalf("staleness changed presence: %q", resource.Presence)
	}
}

type fakeRepository struct{ snapshot InventorySnapshot }

func (f *fakeRepository) Snapshot() (InventorySnapshot, error) { return cloneSnapshot(f.snapshot), nil }
func (f *fakeRepository) CommitProviderSnapshot(expected uint64, s ProviderSnapshot, resources []Resource, changed bool) error {
	if f.snapshot.Revision != expected {
		return ErrSnapshotConflict
	}
	filtered := f.snapshot.Resources[:0]
	for _, resource := range f.snapshot.Resources {
		if resource.Key.Provider != s.ProviderID || resource.Key.ProviderInstance != s.ProviderInstanceID {
			filtered = append(filtered, resource)
		}
	}
	f.snapshot.Resources = append(filtered, resources...)
	if changed {
		f.snapshot.Revision++
	}
	return nil
}
func (f *fakeRepository) RecordProviderFailure(status ProviderStatus) error {
	f.snapshot.Providers = []ProviderStatus{status}
	return nil
}
func cloneSnapshot(snapshot InventorySnapshot) InventorySnapshot {
	result := snapshot
	result.Resources = make([]Resource, len(snapshot.Resources))
	for i, resource := range snapshot.Resources {
		result.Resources[i] = cloneResource(resource)
	}
	result.Providers = append([]ProviderStatus(nil), snapshot.Providers...)
	return result
}

type fakeProvider struct {
	id       ProviderID
	instance ProviderInstanceID
	snapshot ProviderSnapshot
	err      error
}

func (f fakeProvider) ID() ProviderID                                     { return f.id }
func (f fakeProvider) InstanceID() ProviderInstanceID                     { return f.instance }
func (f fakeProvider) Discover(context.Context) (ProviderSnapshot, error) { return f.snapshot, f.err }

func TestCoordinatorProviderFailureDoesNotMarkResourcesMissing(t *testing.T) {
	at := time.Now().UTC()
	repo := &fakeRepository{}
	initial := fakeProvider{id: "docker", instance: "local", snapshot: testSnapshot(at, observation("id", "running"))}
	coordinator, err := NewCoordinator(repo, []DiscoveryProvider{initial}, DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := cloneSnapshot(repo.snapshot)
	failed := fakeProvider{id: "docker", instance: "local", err: errors.New("permission denied")}
	coordinator, err = NewCoordinator(repo, []DiscoveryProvider{failed}, DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderResults[0].ErrorCode != "permission_denied" {
		t.Fatalf("error code=%q", result.ProviderResults[0].ErrorCode)
	}
	if repo.snapshot.Revision != before.Revision || len(repo.snapshot.Resources) != 1 || repo.snapshot.Resources[0].Presence != PresencePresent {
		t.Fatalf("provider failure mutated inventory: %+v", repo.snapshot)
	}
	if repo.snapshot.Providers[0].LastErrorCode != "permission_denied" {
		t.Fatalf("provider status=%+v", repo.snapshot.Providers)
	}
}

func TestNormalizeSnapshotRejectsTerminalControlAndAmbiguousResourceIDs(t *testing.T) {
	at := time.Now().UTC()
	for _, tc := range []struct {
		name     string
		snapshot ProviderSnapshot
	}{
		{name: "slash in resource ID", snapshot: testSnapshot(at, observation("container/other", "running"))},
		{name: "control in display name", snapshot: testSnapshot(at, ResourceObservation{ProviderResourceID: "id", Kind: ResourceContainer, DisplayName: "redis\nunsafe"})},
		{name: "control in attribute value", snapshot: testSnapshot(at, ResourceObservation{ProviderResourceID: "id", Kind: ResourceContainer, Attributes: map[string]string{"safe": "line\nvalue"}})},
		{name: "slash in provider instance", snapshot: ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local/other", Complete: true, ObservedAt: at}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeSnapshot(tc.snapshot); err == nil {
				t.Fatal("unsafe or ambiguous observation accepted")
			}
		})
	}
}

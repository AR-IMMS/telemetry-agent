package domain

import (
	"testing"
	"time"
)

func testObservation(id, state string) ResourceObservation {
	return ResourceObservation{
		ProviderResourceID: id,
		Kind:               ResourceContainer,
		DisplayName:        id,
		LifecycleState:     state,
		Attributes:         map[string]string{"safe": "value"},
		Endpoints:          []Endpoint{{Address: "10.0.0.2", Port: 6379, Protocol: "tcp", Scope: EndpointContainer, Provenance: "container_network"}},
	}
}

func testSnapshot(at time.Time, resources ...ResourceObservation) ProviderSnapshot {
	return ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at, Resources: resources}
}

func TestNormalizeSnapshotDeduplicatesIdenticalRecordsAndDeepCopies(t *testing.T) {
	at := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	item := testObservation("id", "running")
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
	if _, err := NormalizeSnapshot(testSnapshot(at, testObservation("same", "running"), testObservation("same", "exited"))); err == nil {
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

func TestReconciliationTracksMaterialChangesAndFreshnessIndependently(t *testing.T) {
	at := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	policy := Policy{MissingAfterSnapshots: 2, StaleAfter: time.Minute}
	first, changed, err := ReconcileProviderSnapshot(nil, testSnapshot(at, testObservation("id", "running")), policy)
	if err != nil || !changed {
		t.Fatalf("initial reconciliation changed=%v err=%v", changed, err)
	}
	next, changed, err := ReconcileProviderSnapshot(first, testSnapshot(at.Add(30*time.Second), testObservation("id", "running")), policy)
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
	next, changed, err = ReconcileProviderSnapshot(next, testSnapshot(at.Add(2*time.Minute), testObservation("id", "running")), policy)
	if err != nil || !changed || next[0].Presence != PresencePresent || next[0].MissingSnapshots != 0 || next[0].MissingSince != nil {
		t.Fatalf("reappearance changed=%v resource=%+v err=%v", changed, next[0], err)
	}
	now := at.Add(5 * time.Minute)
	if got := next[0].FreshnessAt(now, time.Minute); got != "stale" {
		t.Fatalf("freshness=%q", got)
	}
	if next[0].Presence != PresencePresent {
		t.Fatalf("staleness changed presence: %q", next[0].Presence)
	}
}

func TestNormalizeSnapshotRejectsUnsafeIdentifiersAndFields(t *testing.T) {
	at := time.Now().UTC()
	for _, tc := range []struct {
		name     string
		snapshot ProviderSnapshot
	}{
		{name: "slash in resource ID", snapshot: testSnapshot(at, testObservation("container/other", "running"))},
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

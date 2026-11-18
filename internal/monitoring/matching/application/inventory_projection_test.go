package application

import (
	"testing"
	"time"

	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

func TestProjectInventoryPreservesIdentityAndEndpointEvidence(t *testing.T) {
	asOf := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	lastSeen := asOf.Add(-time.Minute)
	snapshot := inventorydomain.InventorySnapshot{
		Revision: 7,
		Resources: []inventorydomain.Resource{{
			Key:  inventorydomain.ResourceKey{Provider: "docker", ProviderInstance: "desktop", ProviderResourceID: "container-123"},
			Kind: inventorydomain.ResourceContainer, LifecycleState: "running", Presence: inventorydomain.PresencePresent,
			Attributes: map[string]string{"container.image.repository": "library/redis"}, LastSeenAt: lastSeen,
			Endpoints: []inventorydomain.Endpoint{{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: inventorydomain.EndpointHost, Provenance: "published_port"}},
		}},
	}

	got := ProjectInventory(snapshot, asOf)
	if got.InventoryRevision != 7 || !got.AsOf.Equal(asOf) || got.StaleAfter != inventorydomain.DefaultPolicy().StaleAfter {
		t.Fatalf("snapshot metadata = %+v", got)
	}
	if len(got.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(got.Resources))
	}
	resource := got.Resources[0]
	if resource.Provider != "docker" || resource.ProviderInstance != "desktop" || resource.ResourceID != "container-123" ||
		resource.Kind != "container" || resource.Presence != "present" || resource.LifecycleState != "running" || !resource.LastSeenAt.Equal(lastSeen) {
		t.Fatalf("projected resource identity/state = %+v", resource)
	}
	if resource.Attributes["container.image.repository"] != "library/redis" || len(resource.Endpoints) != 1 {
		t.Fatalf("projected resource metadata = %+v", resource)
	}
	endpoint := resource.Endpoints[0]
	if endpoint.Address != "127.0.0.1" || endpoint.Port != 16379 || endpoint.Scope != "host" || endpoint.Provenance != "published_port" {
		t.Fatalf("projected endpoint = %+v", endpoint)
	}
}

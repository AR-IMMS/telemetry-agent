package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
)

func TestIntegrationMatchReadsPersistedInventoryWithoutDesiredStateMutation(t *testing.T) {
	statePath := t.TempDir() + "/agent.db"
	store := agentstate.NewSQLiteStore(statePath)
	repository := store.InventoryRepository()
	now := time.Now().UTC()
	snapshot := inventoryctx.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local-engine", Complete: true, ObservedAt: now,
		Resources: []inventoryctx.ResourceObservation{{
			ProviderResourceID: "container-id", Kind: inventoryctx.ResourceContainer, LifecycleState: "running",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints:  []inventoryctx.Endpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port"}},
		}},
	}
	resource := inventoryctx.Resource{
		Key:  inventoryctx.ResourceKey{Provider: "docker", ProviderInstance: "local-engine", ProviderResourceID: "container-id"},
		Kind: inventoryctx.ResourceContainer, LifecycleState: "running", Presence: inventoryctx.PresencePresent,
		Attributes:  map[string]string{"container.image.repository": "docker.io/library/redis"},
		Endpoints:   []inventoryctx.Endpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port"}},
		FirstSeenAt: now, LastSeenAt: now,
	}
	if err := repository.CommitProviderSnapshot(0, snapshot, []inventoryctx.Resource{resource}, true); err != nil {
		t.Fatal(err)
	}
	desired := store.IntegrationRepository()
	before, err := desired.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(dockerDiscoveryEndpointEnv, "tcp://127.0.0.1:2375")
	var stdout, stderr bytes.Buffer
	if code := runIntegration(context.Background(), []string{"match", "--state-path", statePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("match exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"match_status": "matched"`) || !strings.Contains(stdout.String(), `"definition_id": "redis"`) {
		t.Fatalf("matching output lacks Redis result: %s", stdout.String())
	}
	after, err := desired.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.Instances) != len(before.Instances) {
		t.Fatalf("read-only matching mutated Desired State: before=%+v after=%+v", before, after)
	}
}

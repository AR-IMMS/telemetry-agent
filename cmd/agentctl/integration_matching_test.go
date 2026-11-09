package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

func TestIntegrationMatchReadsPersistedInventoryWithoutDesiredStateMutation(t *testing.T) {
	statePath := t.TempDir() + "/agent.db"
	store := agentstate.NewSQLiteStore(statePath)
	repository := store.InventoryRepository()
	now := time.Now().UTC()
	snapshot := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local-engine", Complete: true, ObservedAt: now,
		Resources: []discovery.ResourceObservation{{
			ProviderResourceID: "container-id", Kind: discovery.ResourceContainer, LifecycleState: "running",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints: []discovery.Endpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port"}},
		}},
	}
	resource := discovery.Resource{
		Key: discovery.ResourceKey{Provider: "docker", ProviderInstance: "local-engine", ProviderResourceID: "container-id"},
		Kind: discovery.ResourceContainer, LifecycleState: "running", Presence: discovery.PresencePresent,
		Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
		Endpoints: []discovery.Endpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port"}},
		FirstSeenAt: now, LastSeenAt: now,
	}
	if err := repository.CommitProviderSnapshot(0, snapshot, []discovery.Resource{resource}, true); err != nil {
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

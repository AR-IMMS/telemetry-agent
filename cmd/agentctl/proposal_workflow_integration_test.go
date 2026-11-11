package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

func TestPersistedInventoryToProposalAndExplicitApproval(t *testing.T) {
	statePath := t.TempDir() + "/state.db"
	store := agentstate.NewSQLiteStore(statePath)
	before := agentstate.State{DesiredGeneration: 4, ActivatedGeneration: 3, AppliedGeneration: 3, Dependencies: map[string]agentstate.DependencyState{"host": {Enabled: true}}}
	if err := store.Save(before); err != nil {
		t.Fatal(err)
	}
	provider := staticRedisInventoryProvider{}
	coordinator, err := discovery.NewCoordinator(store.InventoryRepository(), []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	refresher, err := newRuntimeProposalRefresher(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	proposals, err := store.ProposalRepository().ProposalSnapshot()
	if err != nil || len(proposals) != 1 || proposals[0].Status != discovery.ProposalPending {
		t.Fatalf("proposal snapshot=%+v err=%v", proposals, err)
	}
	var output, stderr strings.Builder
	code := runDiscovery(context.Background(), []string{
		"proposals", "approve", "--state-path", statePath, "--id", proposals[0].ID,
		"--revision", "1", "--expected-desired-revision", "0",
	}, &output, &stderr)
	if code != 0 {
		t.Fatalf("approval command code=%d stderr=%s", code, stderr.String())
	}
	desired, err := store.IntegrationRepository().Snapshot()
	if err != nil || desired.Revision != 1 || len(desired.Instances) != 1 {
		t.Fatalf("desired snapshot=%+v err=%v", desired, err)
	}
	instance := desired.Instances[0]
	if instance.Source != "discovery" || instance.Binding == nil || instance.Binding.ResourceID != "redis-container" || instance.Configuration["endpoint"] != "127.0.0.1:6379" {
		t.Fatalf("approved instance=%+v", instance)
	}
	state, err := store.Load()
	if err != nil || state.DesiredGeneration != before.DesiredGeneration || state.ActivatedGeneration != before.ActivatedGeneration || state.AppliedGeneration != before.AppliedGeneration {
		t.Fatalf("proposal approval changed Collector lifecycle state: %+v err=%v", state, err)
	}
	if !strings.Contains(output.String(), "convergence is pending") {
		t.Fatalf("CLI did not distinguish approval from convergence: %s", output.String())
	}
}

type staticRedisInventoryProvider struct{}

func (staticRedisInventoryProvider) ID() discovery.ProviderID                 { return "docker" }
func (staticRedisInventoryProvider) InstanceID() discovery.ProviderInstanceID { return "local" }
func (staticRedisInventoryProvider) Discover(context.Context) (discovery.ProviderSnapshot, error) {
	at := time.Now().UTC()
	return discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{{
			ProviderResourceID: "redis-container", Kind: discovery.ResourceContainer, DisplayName: "redis-fixture", LifecycleState: "running",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints:  []discovery.Endpoint{{Address: "127.0.0.1", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port"}},
		}},
	}, nil
}

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	proposals "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func TestPersistedInventoryToProposalAndExplicitApproval(t *testing.T) {
	statePath := t.TempDir() + "/state.db"
	store := sqlitefixture.NewSQLiteStore(statePath)
	before := agentstate.State{DesiredGeneration: 4, ActivatedGeneration: 3, AppliedGeneration: 3, Dependencies: map[string]dependencydomain.DependencyState{"host": {Enabled: true}}}
	if err := store.Save(before); err != nil {
		t.Fatal(err)
	}
	provider := staticRedisInventoryProvider{}
	coordinator, err := inventoryapp.NewCoordinator(sqlitefixture.NewInventoryRepositoryFromStore(store), []inventoryapp.DiscoveryProvider{provider}, inventorydomain.DefaultPolicy(), time.Second)
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
	proposalList, err := sqlitefixture.NewProposalRepositoryFromStore(store).ProposalSnapshot()
	if err != nil || len(proposalList) != 1 || proposalList[0].Status != proposals.ProposalPending {
		t.Fatalf("proposal snapshot=%+v err=%v", proposalList, err)
	}
	var output, stderr strings.Builder
	code := runDiscovery(context.Background(), []string{
		"proposals", "approve", "--state-path", statePath, "--id", proposalList[0].ID,
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

func (staticRedisInventoryProvider) ID() inventorydomain.ProviderID                 { return "docker" }
func (staticRedisInventoryProvider) InstanceID() inventorydomain.ProviderInstanceID { return "local" }
func (staticRedisInventoryProvider) Discover(context.Context) (inventorydomain.ProviderSnapshot, error) {
	at := time.Now().UTC()
	return inventorydomain.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []inventorydomain.ResourceObservation{{
			ProviderResourceID: "redis-container", Kind: inventorydomain.ResourceContainer, DisplayName: "redis-fixture", LifecycleState: "running",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints:  []inventorydomain.Endpoint{{Address: "127.0.0.1", Port: 6379, Protocol: "tcp", Scope: inventorydomain.EndpointHost, Provenance: "published_port"}},
		}},
	}, nil
}

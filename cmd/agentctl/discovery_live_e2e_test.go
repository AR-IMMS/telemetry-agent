package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/discovery/docker"
)

// TestLiveDockerInventoryWhenExplicitlyEnabled runs only in the documented local E2E.
func TestLiveDockerInventoryWhenExplicitlyEnabled(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_E2E=1 to run against an isolated live Docker fixture")
	}
	statePath := os.Getenv("AR_IMMS_S4_E2E_STATE")
	fixtureID := os.Getenv("AR_IMMS_S4_E2E_CONTAINER_ID")
	endpoint := os.Getenv(dockerDiscoveryEndpointEnv)
	expectedPresence := os.Getenv("AR_IMMS_S4_E2E_EXPECT_PRESENCE")
	expectedLifecycle := os.Getenv("AR_IMMS_S4_E2E_EXPECT_LIFECYCLE")
	if expectedPresence == "" {
		expectedPresence = string(discovery.PresencePresent)
	}
	if statePath == "" || fixtureID == "" || endpoint == "" {
		t.Fatal("E2E state path, fixture ID, and explicit Docker endpoint are required")
	}
	if filepath.Clean(statePath) == filepath.Clean(agentstate.DefaultPath()) {
		t.Fatal("E2E must not use the production Agent state path")
	}

	store := agentstate.NewSQLiteStore(statePath)
	if err := store.Save(agentstate.State{DesiredGeneration: 3, ActivatedGeneration: 2, AppliedGeneration: 2, Dependencies: map[string]agentstate.DependencyState{"host": {Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	initial, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	desiredBefore, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.InventoryRepository()
	coordinator, err := discovery.NewCoordinator(repository, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cycles := 1
	if expectedPresence == string(discovery.PresenceMissing) {
		cycles = 2
	}
	var result discovery.CycleResult
	for cycle := 0; cycle < cycles; cycle++ {
		result, err = coordinator.DiscoverOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(result.ProviderResults) != 1 || result.ProviderResults[0].ErrorCode != "" {
		t.Fatalf("Docker discovery result=%+v", result.ProviderResults)
	}

	found := false
	for _, resource := range mustInventory(t, repository).Resources {
		if resource.Key.ProviderResourceID != fixtureID {
			continue
		}
		found = true
		if string(resource.Presence) != expectedPresence {
			t.Fatalf("fixture presence=%s, want %s", resource.Presence, expectedPresence)
		}
		if expectedLifecycle != "" && resource.LifecycleState != expectedLifecycle {
			t.Fatalf("fixture lifecycle=%s, want %s", resource.LifecycleState, expectedLifecycle)
		}
	}
	if !found {
		t.Fatalf("fixture %q not found in inventory; missing resources are retained", fixtureID)
	}

	var list, status, stderr bytes.Buffer
	if code := runInventory(context.Background(), []string{"list", "--state-path", statePath}, &list, &stderr); code != 0 {
		t.Fatalf("inventory list code=%d err=%s", code, stderr.String())
	}
	if !strings.Contains(list.String(), fixtureID) || !strings.Contains(list.String(), expectedPresence) {
		t.Fatalf("CLI output lacks fixture evidence: %s", list.String())
	}
	if code := runInventory(context.Background(), []string{"status", "--state-path", statePath}, &status, &stderr); code != 0 {
		t.Fatalf("inventory status code=%d err=%s", code, stderr.String())
	}
	if !strings.Contains(status.String(), "last error: none") {
		t.Fatalf("provider status=%s", status.String())
	}

	if expectedPresence == string(discovery.PresencePresent) {
		stable := false
		revision := result.InventoryRevision
		for attempt := 0; attempt < 6; attempt++ {
			time.Sleep(300 * time.Millisecond)
			next, err := coordinator.DiscoverOnce(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if next.InventoryRevision == revision {
				stable = true
				break
			}
			revision = next.InventoryRevision
		}
		if !stable {
			t.Fatal("inventory did not reach a stable material revision across repeated Docker snapshots")
		}
	}

	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	desiredAfter, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.DesiredGeneration != initial.DesiredGeneration || after.ActivatedGeneration != initial.ActivatedGeneration || after.AppliedGeneration != initial.AppliedGeneration || desiredAfter.Revision != desiredBefore.Revision {
		t.Fatalf("discovery changed S1-S3 lifecycle state: before=%+v after=%+v desired %d -> %d", initial, after, desiredBefore.Revision, desiredAfter.Revision)
	}
}

func TestLiveDockerUnavailableRetainsInventory(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_E2E=1 to run provider failure verification")
	}
	statePath := os.Getenv("AR_IMMS_S4_E2E_STATE")
	endpoint := os.Getenv("AR_IMMS_S4_E2E_UNAVAILABLE_ENDPOINT")
	if statePath == "" || endpoint == "" {
		t.Fatal("E2E state path and unreachable local Docker endpoint are required")
	}
	store := agentstate.NewSQLiteStore(statePath)
	repository := store.InventoryRepository()
	before, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := discovery.NewCoordinator(repository, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProviderResults) != 1 || result.ProviderResults[0].ErrorCode == "" {
		t.Fatalf("expected safe provider error, got %+v", result.ProviderResults)
	}
	after, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision || len(before.Resources) != len(after.Resources) {
		t.Fatalf("provider failure changed inventory: before=%+v after=%+v", before, after)
	}
	providerStatusFound := false
	for _, status := range after.Providers {
		if status.ProviderID == provider.ID() && status.ProviderInstanceID == provider.InstanceID() {
			providerStatusFound = status.LastErrorCode != ""
		}
	}
	if !providerStatusFound {
		t.Fatalf("provider failure was not recorded: %+v", after.Providers)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 3 || state.ActivatedGeneration != 2 || state.AppliedGeneration != 2 {
		t.Fatalf("provider failure changed lifecycle generations: %+v", state)
	}
}

func mustInventory(t *testing.T, repository discovery.InventoryRepository) discovery.InventorySnapshot {
	t.Helper()
	snapshot, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

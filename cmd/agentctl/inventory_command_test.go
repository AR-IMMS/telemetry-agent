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
)

func TestInventoryCommandsReadPersistedSnapshot(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	repository := agentstate.NewSQLiteStore(statePath).InventoryRepository()
	at := time.Now().UTC()
	snapshot := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{{
			ProviderResourceID: "container-1", Kind: discovery.ResourceContainer,
			DisplayName: "redis-test", LifecycleState: "running",
			Attributes: map[string]string{"label.safe": "value", "label.ar-imms.monitoring.enabled": "true", "label.ar-imms.monitoring.profile": "redis"},
			Endpoints: []discovery.Endpoint{
				{Address: "172.18.0.2", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
				{Address: "127.0.0.1", Port: 16379, ContainerPort: 6379, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port"},
			},
		}},
	}
	resources, changed, err := discovery.ReconcileProviderSnapshot(nil, snapshot, discovery.DefaultPolicy())
	if err != nil || !changed {
		t.Fatalf("reconcile changed=%v err=%v", changed, err)
	}
	if err := repository.CommitProviderSnapshot(0, snapshot, resources, changed); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"list", "--state-path", statePath}, []string{"docker", "container-1", "redis-test", "running", "present", "fresh"}},
		{[]string{"inspect", "--state-path", statePath, "docker/local/container-1"}, []string{"Endpoint evidence:", "172.18.0.2", "container_network", "label.safe=value", "Monitoring profile: redis", "Profile endpoint: endpoint_resolved", "Endpoint reachability: unverified"}},
		{[]string{"status", "--state-path", statePath}, []string{"Inventory revision: 1", "Provider: docker/local", "resources: 1", "last error: none"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := runInventory(context.Background(), tc.args, &stdout, &stderr); code != 0 {
			t.Fatalf("runInventory(%v) code=%d stderr=%s", tc.args, code, stderr.String())
		}
		for _, fragment := range tc.want {
			if !strings.Contains(stdout.String(), fragment) {
				t.Errorf("runInventory(%v) output missing %q: %s", tc.args, fragment, stdout.String())
			}
		}
		if len(tc.args) > 0 && tc.args[0] == "inspect" && !strings.Contains(stdout.String(), "label.safe=value\nEndpoint evidence:") {
			t.Errorf("inspect attributes must end with a line break before endpoint evidence: %q", stdout.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("runInventory(%v) stderr=%s", tc.args, stderr.String())
		}
	}
}

func TestInventoryListActiveOnlyExcludesMissingHistory(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	repository := agentstate.NewSQLiteStore(statePath).InventoryRepository()
	at := time.Now().UTC()
	first := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{
			{ProviderResourceID: "redis-v1", Kind: discovery.ResourceContainer, DisplayName: "redis-old", LifecycleState: "running"},
			{ProviderResourceID: "redis-v2", Kind: discovery.ResourceContainer, DisplayName: "redis-new", LifecycleState: "running"},
		},
	}
	resources, changed, err := discovery.ReconcileProviderSnapshot(nil, first, discovery.DefaultPolicy())
	if err != nil || !changed {
		t.Fatalf("first snapshot changed=%v err=%v", changed, err)
	}
	if err := repository.CommitProviderSnapshot(0, first, resources, changed); err != nil {
		t.Fatal(err)
	}
	for miss := 1; miss <= 2; miss++ {
		snapshot := discovery.ProviderSnapshot{
			ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at.Add(time.Duration(miss) * time.Second),
			Resources: []discovery.ResourceObservation{{ProviderResourceID: "redis-v2", Kind: discovery.ResourceContainer, DisplayName: "redis-new", LifecycleState: "running"}},
		}
		current, err := repository.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		resources, changed, err = discovery.ReconcileProviderSnapshot(current.Resources, snapshot, discovery.DefaultPolicy())
		if err != nil {
			t.Fatalf("miss %d changed=%v err=%v", miss, changed, err)
		}
		if changed != (miss == 2) {
			t.Fatalf("miss %d material revision changed=%v, want %t", miss, changed, miss == 2)
		}
		if err := repository.CommitProviderSnapshot(current.WriteRevision, snapshot, resources, changed); err != nil {
			t.Fatal(err)
		}
	}

	var activeOut, activeErr bytes.Buffer
	if code := runInventory(context.Background(), []string{"list", "--active-only", "--state-path", statePath}, &activeOut, &activeErr); code != 0 {
		t.Fatalf("active-only list code=%d stderr=%s", code, activeErr.String())
	}
	if !strings.Contains(activeOut.String(), "redis-v2") || strings.Contains(activeOut.String(), "redis-v1") {
		t.Fatalf("active-only view should show only the recreated container: %s", activeOut.String())
	}

	var historyOut, historyErr bytes.Buffer
	if code := runInventory(context.Background(), []string{"list", "--state-path", statePath}, &historyOut, &historyErr); code != 0 {
		t.Fatalf("history list code=%d stderr=%s", code, historyErr.String())
	}
	if !strings.Contains(historyOut.String(), "redis-v1") || !strings.Contains(historyOut.String(), "missing") {
		t.Fatalf("default inventory history should retain the old missing container: %s", historyOut.String())
	}
}

func TestInventoryCommandsRejectInvalidArgumentsAndHonorCanceledContext(t *testing.T) {
	invalidDatabasePath := filepath.Join(t.TempDir(), "not-a-database.db")
	if err := os.WriteFile(invalidDatabasePath, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"list", "--state-path", invalidDatabasePath, "unexpected"},
		{"inspect", "--state-path", invalidDatabasePath},
		{"unknown", "--state-path", invalidDatabasePath},
		{"list", "--state-path", " "},
		{"status", "--state-path", filepath.Join(t.TempDir(), "state.db"), "--active-only"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runInventory(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Errorf("runInventory(%v) code=%d want 2, stderr=%s", args, code, stderr.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := runInventory(ctx, []string{"status", "--state-path", filepath.Join(t.TempDir(), "state.db")}, &stdout, &stderr); code != 1 {
		t.Fatalf("canceled command code=%d, stderr=%s", code, stderr.String())
	}
}

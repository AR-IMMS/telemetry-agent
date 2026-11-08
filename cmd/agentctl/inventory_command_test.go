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
	at := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	snapshot := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{{
			ProviderResourceID: "container-1", Kind: discovery.ResourceContainer,
			DisplayName: "redis-test", LifecycleState: "running",
			Attributes: map[string]string{"label.safe": "value"},
			Endpoints:  []discovery.Endpoint{{Address: "172.18.0.2", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"}},
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
		{[]string{"inspect", "--state-path", statePath, "docker/local/container-1"}, []string{"Endpoint evidence:", "172.18.0.2", "container_network", "label.safe=value"}},
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
		if stderr.Len() != 0 {
			t.Errorf("runInventory(%v) stderr=%s", tc.args, stderr.String())
		}
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

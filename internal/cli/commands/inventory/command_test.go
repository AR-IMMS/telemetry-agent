package inventory

import (
	"bytes"
	"context"
	"testing"

	domain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

func TestExecuteListUsesInjectedSnapshotAndPreservesColumns(t *testing.T) {
	var loadedPath string
	deps := Dependencies{
		DefaultStatePath: func() string { return "default.db" },
		LoadSnapshot: func(path string) (domain.InventorySnapshot, error) {
			loadedPath = path
			return domain.InventorySnapshot{Revision: 8, Resources: []domain.Resource{{
				Key:  domain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-1"},
				Kind: domain.ResourceContainer, DisplayName: "redis", LifecycleState: "running", Presence: domain.PresencePresent,
			}}, Providers: []domain.ProviderStatus{}}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"list"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("Execute() = %d, stderr=%q", code, stderr.String())
	}
	if loadedPath != "default.db" {
		t.Fatalf("snapshot path = %q, want default.db", loadedPath)
	}
	for _, want := range []string{"PROVIDER\tRESOURCE\tKIND\tNAME\tSTATE\tPRESENCE\tFRESHNESS", "docker\tdocker/local/container-1\tcontainer\tredis\trunning\tpresent"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Errorf("output %q does not contain %q", stdout.String(), want)
		}
	}
}

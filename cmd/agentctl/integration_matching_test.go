package main

import (
	"bytes"
	"context"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCatalogCapabilityAssessorUsesPinnedRuntimeCatalog(t *testing.T) {
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	catalog, err := collectorruntime.StaticCatalog(platform)
	if err != nil {
		t.Fatalf("load Runtime catalog for %s/%s: %v", platform.OS, platform.Architecture, err)
	}
	assessor := integration.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)

	assessment, err := assessor.Assess([]string{monitoringdomain.RedisCapability})
	if err != nil {
		t.Fatalf("assess Redis capability: %v", err)
	}
	if assessment.Status != integration.CapabilitySupported || assessment.Platform != platform.OS+"/"+platform.Architecture ||
		assessment.ArtifactVersion != "0.160.0" || assessment.ManifestVersion == "" {
		t.Fatalf("Redis assessment lacks exact pinned Runtime evidence: %+v", assessment)
	}

	assessment, err = assessor.Assess([]string{monitoringdomain.RedisCapability, "otel.receiver.not_in_pinned_catalog.metrics"})
	if err != nil {
		t.Fatalf("assess unsupported capability: %v", err)
	}
	if assessment.Status != integration.CapabilityUnsupported || len(assessment.Unsupported) != 1 ||
		assessment.Unsupported[0] != "otel.receiver.not_in_pinned_catalog.metrics" {
		t.Fatalf("unsupported capability assessment = %+v", assessment)
	}
}

func TestIntegrationMatchReadsPersistedInventoryWithoutDesiredStateMutation(t *testing.T) {
	statePath := t.TempDir() + "/agent.db"
	store := sqlitefixture.NewSQLiteStore(statePath)
	repository := store.InventoryRepository()
	now := time.Now().UTC()
	snapshot := inventorydomain.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local-engine", Complete: true, ObservedAt: now,
		Resources: []inventorydomain.ResourceObservation{{
			ProviderResourceID: "container-id", Kind: inventorydomain.ResourceContainer, LifecycleState: "running",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints:  []inventorydomain.Endpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: inventorydomain.EndpointHost, Provenance: "published_port"}},
		}},
	}
	resource := inventorydomain.Resource{
		Key:  inventorydomain.ResourceKey{Provider: "docker", ProviderInstance: "local-engine", ProviderResourceID: "container-id"},
		Kind: inventorydomain.ResourceContainer, LifecycleState: "running", Presence: inventorydomain.PresencePresent,
		Attributes:  map[string]string{"container.image.repository": "docker.io/library/redis"},
		Endpoints:   []inventorydomain.Endpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: inventorydomain.EndpointHost, Provenance: "published_port"}},
		FirstSeenAt: now, LastSeenAt: now,
	}
	if err := repository.CommitProviderSnapshot(0, snapshot, []inventorydomain.Resource{resource}, true); err != nil {
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

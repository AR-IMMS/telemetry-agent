package main

import (
	"context"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/inventory/docker"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	templatesmonitoringapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
)

// TestLiveDockerTemplateAutoMaterializesDesiredInstance verifies the live Docker
// provider creates a Template-owned desired instance without proposal approval.
func TestLiveDockerTemplateAutoMaterializesDesiredInstance(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_TEMPLATE_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_TEMPLATE_E2E=1 to run against an isolated live Docker fixture")
	}
	statePath := os.Getenv("AR_IMMS_S4_TEMPLATE_E2E_STATE")
	fixtureID := os.Getenv("AR_IMMS_S4_TEMPLATE_E2E_CONTAINER_ID")
	endpoint := os.Getenv(dockerDiscoveryEndpointEnv)
	if statePath == "" || fixtureID == "" || endpoint == "" {
		t.Fatal("isolated state path, Redis container ID, and explicit Docker endpoint are required")
	}
	if filepath.Clean(statePath) == filepath.Clean(agentstate.DefaultPath()) {
		t.Fatal("E2E must not use the production Agent state path")
	}
	relative, err := filepath.Rel(os.TempDir(), filepath.Clean(statePath))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("E2E state database must be inside the temporary directory")
	}

	store := sqlitefixture.NewSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	templateRepository := store.TemplateRepository()
	templateService := templatesapp.NewService(registry, templateRepository)
	if _, _, err := templateService.Put(redisRuntimeTemplate(), 0); err != nil {
		t.Fatal(err)
	}
	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := inventoryapp.NewCoordinator(sqlitefixture.NewInventoryRepositoryFromStore(store), []inventoryapp.DiscoveryProvider{provider}, inventorydomain.DefaultPolicy(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := coordinator.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !cycle.Succeeded() {
		t.Fatalf("Docker provider cycle failed: %+v", cycle.ProviderResults)
	}
	inventory, err := sqlitefixture.NewInventoryRepositoryFromStore(store).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	fixtureFound := false
	for _, resource := range inventory.Resources {
		if resource.Key.ProviderResourceID == fixtureID {
			fixtureFound = true
			if resource.Kind != inventorydomain.ResourceContainer || resource.Presence != inventorydomain.PresencePresent || resource.Attributes["container.image.repository"] != "docker.io/library/redis" {
				t.Fatalf("fixture is not a present Redis container: %+v", resource)
			}
		}
	}
	if !fixtureFound {
		t.Fatalf("Redis fixture %q was not found in the complete Docker inventory", fixtureID)
	}

	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := collectorruntime.StaticCatalog(platform)
	if err != nil {
		t.Fatal(err)
	}
	materializer := runtimeTemplateMaterializer{
		inventory: sqlitefixture.NewInventoryRepositoryFromStore(store), templates: templateRepository, desired: newTestDesiredStateRepository(store),
		service: materializationapp.NewService(store.MaterializationRepository()),
		matcher: templatesmonitoringapp.NewEvaluator(registry, matchingdomain.NewCatalogCapabilityAssessor(catalog, runtime.GOOS, runtime.GOOS+"/"+runtime.GOARCH)),
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	desired, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var generated *desiredstatedomain.Instance
	for index := range desired.Instances {
		instance := &desired.Instances[index]
		if instance.Source == desiredstatedomain.SourceTemplate && instance.Binding != nil && instance.Binding.ResourceID == fixtureID {
			generated = instance
			break
		}
	}
	if generated == nil || !generated.Enabled || generated.TemplateID != "redis-auto" || generated.TemplateOwner != desiredstatedomain.TemplateOwnerLocal {
		t.Fatalf("live Docker observation did not create the enabled Template-owned desired instance: %+v", desired)
	}
	proposalRefresher, err := newRuntimeProposalRefresher(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := proposalRefresher.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	proposals, err := sqlitefixture.NewProposalRepositoryFromStore(store).ProposalSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, proposal := range proposals {
		if proposal.Identity.ResourceID == fixtureID && proposal.Identity.IntegrationID == "redis" {
			t.Fatalf("auto-materializable Redis candidate also became a manual approval proposal: %+v", proposal)
		}
	}
}

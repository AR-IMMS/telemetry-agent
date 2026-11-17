package main

import (
	"context"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	templatesconfigdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	templatesmonitoringapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"path/filepath"
	"testing"
	"time"
)

type supportedTemplateCapability struct{}

func (supportedTemplateCapability) Assess(required []string) (matchingdomain.CapabilityAssessment, error) {
	return matchingdomain.CapabilityAssessment{Status: matchingdomain.CapabilitySupported, Platform: "windows/amd64", ArtifactVersion: "test", ManifestVersion: "test", Required: required}, nil
}

type mutableTemplateProvider struct {
	snapshot inventorydomain.ProviderSnapshot
	err      error
}

func (p *mutableTemplateProvider) ID() inventorydomain.ProviderID                 { return "docker" }
func (p *mutableTemplateProvider) InstanceID() inventorydomain.ProviderInstanceID { return "local" }
func (p *mutableTemplateProvider) Discover(context.Context) (inventorydomain.ProviderSnapshot, error) {
	return p.snapshot, p.err
}

func TestRuntimeTemplateMaterializerProjectsStableInstancesAndNoOps(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	templates := newTestTemplateRepository(store)
	service := templatesapp.NewService(registry, templates)
	if _, _, err := service.Put(redisRuntimeTemplate(), 0); err != nil {
		t.Fatal(err)
	}
	provider := &mutableTemplateProvider{snapshot: redisProviderSnapshot("container-a")}
	coordinator, err := inventoryapp.NewCoordinator(sqlitefixture.NewInventoryRepositoryFromStore(store), []inventoryapp.DiscoveryProvider{provider}, inventorydomain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	materializer := runtimeTemplateMaterializer{
		inventory: sqlitefixture.NewInventoryRepositoryFromStore(store), templates: templates, desired: newTestDesiredStateRepository(store),
		service: materializationapp.NewService(store.MaterializationRepository()),
		matcher: templatesmonitoringapp.NewEvaluator(registry, supportedTemplateCapability{}),
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || first.Revision != 1 || len(first.Instances) != 1 || !first.Instances[0].Enabled {
		t.Fatalf("first desired snapshot=%+v err=%v", first, err)
	}
	stableID := first.Instances[0].ID
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	unchanged, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || unchanged.Revision != first.Revision || unchanged.Instances[0].Revision != first.Instances[0].Revision {
		t.Fatalf("identical projection changed desired state: %+v err=%v", unchanged, err)
	}

	provider.snapshot = redisProviderSnapshot("container-a", "container-b")
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	multiple, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || multiple.Revision != 2 || len(multiple.Instances) != 2 {
		t.Fatalf("multi-resource desired snapshot=%+v err=%v", multiple, err)
	}
	for _, instance := range multiple.Instances {
		if instance.ID == stableID && !instance.Enabled {
			t.Fatalf("stable resource identity was not preserved: %+v", instance)
		}
	}

	if _, _, err := service.SetEnabled("redis-auto", false, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	disabled, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || disabled.Revision != 3 || len(disabled.Instances) != 2 {
		t.Fatalf("disabled desired snapshot=%+v err=%v", disabled, err)
	}
	for _, instance := range disabled.Instances {
		if instance.Enabled || instance.Source != desiredstatedomain.SourceTemplate {
			t.Fatalf("disabled Template did not suspend only generated instances: %+v", instance)
		}
	}
	if _, _, err := service.SetEnabled("redis-auto", true, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	reenabled, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil || reenabled.Revision != 4 || len(reenabled.Instances) != 2 {
		t.Fatalf("re-enabled desired snapshot=%+v err=%v", reenabled, err)
	}
	for _, instance := range reenabled.Instances {
		if !instance.Enabled || instance.Source != desiredstatedomain.SourceTemplate {
			t.Fatalf("re-enabled Template did not restore generated instances: %+v", instance)
		}
	}
}

func TestNativeRedisProfileDoesNotCreateAgentInstancesDuringContainerChurn(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	templates := newTestTemplateRepository(store)
	service := templatesapp.NewService(registry, templates)
	if _, _, err := service.Put(redisRuntimeTemplate(), 0); err != nil {
		t.Fatal(err)
	}
	provider := &mutableTemplateProvider{snapshot: nativeRedisProviderSnapshot("container-a")}
	coordinator, err := inventoryapp.NewCoordinator(sqlitefixture.NewInventoryRepositoryFromStore(store), []inventoryapp.DiscoveryProvider{provider}, inventorydomain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	materializer := runtimeTemplateMaterializer{
		inventory: sqlitefixture.NewInventoryRepositoryFromStore(store), templates: templates, desired: newTestDesiredStateRepository(store),
		service:          materializationapp.NewService(store.MaterializationRepository()),
		matcher:          templatesmonitoringapp.NewEvaluator(registry, supportedTemplateCapability{}),
		nativeMonitoring: true, nativeProfiles: monitoringdomain.DefaultRegistry(),
	}
	for _, ids := range [][]string{{"container-a"}, {"container-a", "container-b"}, {"container-b"}, {"container-a-recreated", "container-b"}} {
		provider.snapshot = nativeRedisProviderSnapshot(ids...)
		if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := materializer.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	desired, err := newTestDesiredStateRepository(store).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired.Instances) != 0 || desired.Revision != 0 {
		t.Fatalf("native Redis container churn changed Agent desired state: %+v", desired)
	}
}

func nativeRedisProviderSnapshot(ids ...string) inventorydomain.ProviderSnapshot {
	resources := make([]inventorydomain.ResourceObservation, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, inventorydomain.ResourceObservation{
			ProviderResourceID: id, Kind: inventorydomain.ResourceContainer, LifecycleState: "running",
			Attributes: map[string]string{
				"container.image.repository":       "docker.io/library/redis",
				"label.ar-imms.monitoring.enabled": "true",
				"label.ar-imms.monitoring.profile": "redis",
				"label.ar-imms.workload.id":        "cache-service",
			},
			Endpoints: []inventorydomain.Endpoint{{Address: "127.0.0.1", Port: 6379, Protocol: "tcp", Scope: inventorydomain.EndpointHost, Provenance: "published_port"}},
		})
	}
	return inventorydomain.ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, Resources: resources}
}

func TestRuntimeTemplateUpdateChangesGeneratedInstancesButPreservesManualState(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	templates := newTestTemplateRepository(store)
	templateService := templatesapp.NewService(registry, templates)
	template := redisRuntimeTemplate()
	if _, _, err := templateService.Put(template, 0); err != nil {
		t.Fatal(err)
	}
	provider := &mutableTemplateProvider{snapshot: redisProviderSnapshot("container-a", "container-b")}
	coordinator, err := inventoryapp.NewCoordinator(sqlitefixture.NewInventoryRepositoryFromStore(store), []inventoryapp.DiscoveryProvider{provider}, inventorydomain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	desiredRepository := newTestDesiredStateRepository(store)
	materializer := runtimeTemplateMaterializer{
		inventory: sqlitefixture.NewInventoryRepositoryFromStore(store), templates: templates, desired: desiredRepository,
		service: materializationapp.NewService(store.MaterializationRepository()),
		matcher: templatesmonitoringapp.NewEvaluator(registry, supportedTemplateCapability{}),
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	manual, err := (desiredStateResolverAdapter{resolver: definitionapp.NewResolver(registry)}).Resolve(desiredstatedomain.Instance{
		ID: "manual-redis", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true,
		Configuration: map[string]any{"endpoint": "192.0.2.10:6379"}, Source: desiredstatedomain.SourceLocal,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeUpdate, err := desiredRepository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	storedManual, _, err := desiredRepository.Put(manual, 0)
	if err != nil {
		t.Fatal(err)
	}
	if storedManual.Revision == 0 {
		t.Fatal("manual instance has no persisted revision")
	}

	template.Settings = map[string]any{"collection_interval": "15s"}
	if _, _, err := templateService.Put(template, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := materializer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	afterUpdate, err := desiredRepository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if afterUpdate.Revision != beforeUpdate.Revision+2 || len(afterUpdate.Instances) != 3 {
		t.Fatalf("updated desired snapshot=%+v; before=%+v", afterUpdate, beforeUpdate)
	}
	manualPreserved := false
	for _, instance := range afterUpdate.Instances {
		if instance.ID == storedManual.ID {
			manualPreserved = instance.Revision == storedManual.Revision && instance.Source == desiredstatedomain.SourceLocal && instance.Configuration["endpoint"] == "192.0.2.10:6379"
			continue
		}
		if instance.Source != desiredstatedomain.SourceTemplate || instance.TemplateID != template.ID || instance.Configuration["collection_interval"] != "15s" || instance.Revision != 2 {
			t.Fatalf("Template update did not update only its generated instance: %+v", instance)
		}
	}
	if !manualPreserved {
		t.Fatal("Template update changed or removed the manual Integration instance")
	}
}

func TestRuntimeTemplateMaterializerPreservesFailureAndTracksResourceLifecycle(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	templates := newTestTemplateRepository(store)
	templateService := templatesapp.NewService(registry, templates)
	if _, _, err := templateService.Put(redisRuntimeTemplate(), 0); err != nil {
		t.Fatal(err)
	}
	provider := &mutableTemplateProvider{snapshot: redisProviderSnapshot("container-a")}
	coordinator, err := inventoryapp.NewCoordinator(sqlitefixture.NewInventoryRepositoryFromStore(store), []inventoryapp.DiscoveryProvider{provider}, inventorydomain.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	desiredRepository := newTestDesiredStateRepository(store)
	materializer := runtimeTemplateMaterializer{
		inventory: sqlitefixture.NewInventoryRepositoryFromStore(store), templates: templates, desired: desiredRepository,
		service: materializationapp.NewService(store.MaterializationRepository()),
		matcher: templatesmonitoringapp.NewEvaluator(registry, supportedTemplateCapability{}),
	}
	refreshAfterSuccessfulCycle := func() {
		t.Helper()
		cycle, err := coordinator.DiscoverOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if cycle.Succeeded() {
			if err := materializer.Refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
	}
	readDesired := func() desiredstateapp.Snapshot {
		t.Helper()
		snapshot, err := desiredRepository.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}

	refreshAfterSuccessfulCycle()
	created := readDesired()
	if created.Revision != 1 || len(created.Instances) != 1 || !created.Instances[0].Enabled {
		t.Fatalf("created desired snapshot=%+v", created)
	}
	instanceID := created.Instances[0].ID

	provider.err = context.DeadlineExceeded
	refreshAfterSuccessfulCycle()
	afterFailure := readDesired()
	if afterFailure.Revision != created.Revision || !afterFailure.Instances[0].Enabled {
		t.Fatalf("provider failure changed generated desired state: %+v", afterFailure)
	}

	provider.err = nil
	provider.snapshot = redisProviderSnapshot()
	refreshAfterSuccessfulCycle()
	afterFirstMiss := readDesired()
	if afterFirstMiss.Revision != created.Revision || !afterFirstMiss.Instances[0].Enabled {
		t.Fatalf("first complete miss changed generated desired state: %+v", afterFirstMiss)
	}
	refreshAfterSuccessfulCycle()
	afterConfirmedAbsence := readDesired()
	if afterConfirmedAbsence.Revision != created.Revision+1 || len(afterConfirmedAbsence.Instances) != 1 || afterConfirmedAbsence.Instances[0].Enabled || afterConfirmedAbsence.Instances[0].ID != instanceID {
		t.Fatalf("confirmed absence did not suspend and retain the generated instance: %+v", afterConfirmedAbsence)
	}

	provider.snapshot = redisProviderSnapshot("container-a")
	refreshAfterSuccessfulCycle()
	afterReappearance := readDesired()
	if afterReappearance.Revision != afterConfirmedAbsence.Revision+1 || len(afterReappearance.Instances) != 1 || !afterReappearance.Instances[0].Enabled || afterReappearance.Instances[0].ID != instanceID {
		t.Fatalf("resource reappearance did not restore the stable instance: %+v", afterReappearance)
	}

	provider.snapshot = redisProviderSnapshot("container-b")
	refreshAfterSuccessfulCycle()
	afterFirstReplacementMiss := readDesired()
	newID, err := materializationdomain.GeneratedInstanceID(desiredstatedomain.TemplateOwnerLocal, "redis-auto", desiredstatedomain.ResourceBinding{
		Provider: "docker", ProviderInstance: "local", ResourceID: "container-b",
	}, "redis")
	if err != nil {
		t.Fatal(err)
	}
	if afterFirstReplacementMiss.Revision != afterReappearance.Revision+1 || len(afterFirstReplacementMiss.Instances) != 2 {
		t.Fatalf("first replacement miss desired snapshot=%+v", afterFirstReplacementMiss)
	}
	oldEnabled, newEnabled := false, false
	for _, instance := range afterFirstReplacementMiss.Instances {
		if instance.ID == instanceID {
			oldEnabled = instance.Enabled
		}
		if instance.ID == newID {
			newEnabled = instance.Enabled
		}
	}
	if !oldEnabled || !newEnabled {
		t.Fatalf("two-snapshot missing policy should retain old and new instances through the first miss: %+v", afterFirstReplacementMiss.Instances)
	}

	refreshAfterSuccessfulCycle()
	afterReplacement := readDesired()
	if afterReplacement.Revision != afterFirstReplacementMiss.Revision+1 || len(afterReplacement.Instances) != 2 {
		t.Fatalf("replacement desired snapshot=%+v", afterReplacement)
	}
	oldSuspended, newEnabled := false, false
	for _, instance := range afterReplacement.Instances {
		if instance.ID == instanceID {
			oldSuspended = !instance.Enabled
		}
		if instance.ID == newID {
			newEnabled = instance.Enabled
		}
	}
	if !oldSuspended || !newEnabled || newID == instanceID {
		t.Fatalf("resource recreation did not preserve old identity and create a distinct active instance: %+v", afterReplacement.Instances)
	}

	if _, _, err := templateService.Delete("redis-auto", 1); err != nil {
		t.Fatal(err)
	}
	refreshAfterSuccessfulCycle()
	afterDelete := readDesired()
	if afterDelete.Revision != afterReplacement.Revision+1 || len(afterDelete.Instances) != 2 {
		t.Fatalf("deleted Template desired snapshot=%+v", afterDelete)
	}
	for _, instance := range afterDelete.Instances {
		if instance.Enabled {
			t.Fatalf("deleted Template left a generated instance enabled: %+v", instance)
		}
	}
}

func redisRuntimeTemplate() templatesconfigdomain.Template {
	return templatesconfigdomain.Template{
		ID: "redis-auto", DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: desiredstatedomain.TemplateOwnerLocal,
		Revision: 1, Status: templatesconfigdomain.StatusEnabled,
		Selector: &matchingdomain.MatchSelector{ResourceKinds: []string{"container"}, ExactAttributes: map[string]string{"container.image.repository": "docker.io/library/redis"}},
		Bindings: []templatesconfigdomain.SettingBinding{{Target: "endpoint", Source: templatesconfigdomain.BindingSource{Kind: templatesconfigdomain.SourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port"}}},
	}
}

func redisProviderSnapshot(ids ...string) inventorydomain.ProviderSnapshot {
	resources := make([]inventorydomain.ResourceObservation, 0, len(ids))
	for index, id := range ids {
		resources = append(resources, inventorydomain.ResourceObservation{
			ProviderResourceID: id, Kind: inventorydomain.ResourceContainer, LifecycleState: "running",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints:  []inventorydomain.Endpoint{{Address: "127.0.0.1", Port: uint16(6379 + index), Protocol: "tcp", Scope: inventorydomain.EndpointHost, Provenance: "published_port"}},
		})
	}
	return inventorydomain.ProviderSnapshot{ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: time.Now().UTC(), Resources: resources}
}

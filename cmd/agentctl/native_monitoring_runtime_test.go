package main

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	installationapp "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	collectorruntime "github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	config "github.com/ar-imms/telemetry-agent/internal/runtime/collector/configuration"
)

func TestPinnedCollectorValidatesNativeDockerRedisProfile(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if binary == "" {
		t.Skip("set AR_IMMS_PR05_COLLECTOR_BINARY to validate against pinned Collector v0.160.0")
	}
	t.Setenv("OTEL_GATEWAY_ENDPOINT", "127.0.0.1:4317")
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatal(err)
	}
	configRoot, err := filepath.Abs(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	layers, err := configurationLayers(configRoot, platform.OS)
	if err != nil {
		t.Fatal(err)
	}
	dockerEndpoint := "unix:///var/run/docker.sock"
	if platform.OS == "windows" {
		dockerEndpoint = "npipe:////./pipe/docker_engine"
	}
	profileLayer, receiverNames, err := collectorruntime.DockerMonitoringLayer(collectorMonitoringProfiles(monitoringdomain.DefaultRegistry().Profiles()), platform.OS, dockerEndpoint, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	dependencyCatalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	receiverLayer, err := dependencyMetricsReceiverLayer(dependencyCatalog, platform.OS, agentstate.State{}, receiverNames...)
	if err != nil {
		t.Fatal(err)
	}
	input := config.RenderInput{Platform: platform, Layers: layers, InlineLayers: []config.InlineLayer{receiverLayer, profileLayer}, Values: map[string]string{"OTEL_GATEWAY_ENDPOINT": "127.0.0.1:4317"}}
	rendered, err := config.Render(input)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "native-redis.yaml")
	if err := os.WriteFile(configPath, rendered, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), binary, "validate", "--config", configPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pinned Collector rejected native Docker Redis profile: %v\n%s\n%s", err, output, rendered)
	}
}

func TestPinnedCollectorValidatesLocalPostgreSQLAndPrometheusProfiles(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("OTELCOL_CONTRIB_BINARY"))
	if binary == "" {
		t.Skip("set OTELCOL_CONTRIB_BINARY to validate profiles against pinned Collector v0.160.0")
	}
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := bootstrap.SelectArtifact(platform)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(os.Getenv("OTELCOL_ARTIFACT_SHA256")); got != artifact.ArchiveSHA256 {
		t.Fatalf("pinned Collector artifact checksum=%q, want %q", got, artifact.ArchiveSHA256)
	}
	t.Setenv("OTEL_GATEWAY_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("AR_IMMS_TEST_PG_USER", "metrics_reader")
	t.Setenv("AR_IMMS_TEST_PG_PASSWORD", "fixture-only-secret")
	root := t.TempDir()
	profilesDir := filepath.Join(root, "monitoring-profiles")
	if err := os.MkdirAll(profilesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := `api_version: ar-imms/v1alpha1
profiles:
  - id: postgres
    provider: docker
    receiver_type: postgresql
    required_capabilities: [otel.receiver.postgresql.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: postgres}
    collection_interval: 30s
    target_port: 5432
    target_port_label: ar-imms.monitoring.port
    username_env: AR_IMMS_TEST_PG_USER
    password_env: AR_IMMS_TEST_PG_PASSWORD
  - id: python-metrics
    provider: docker
    receiver_type: prometheus
    required_capabilities: [otel.receiver.prometheus.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: python-metrics}
    collection_interval: 15s
    target_port: 9108
    target_port_label: ar-imms.monitoring.port
    metrics_path: /metrics
`
	if err := os.WriteFile(filepath.Join(profilesDir, "test.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := loadMonitoringProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	configRoot, err := filepath.Abs(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	layers, err := configurationLayers(configRoot, platform.OS)
	if err != nil {
		t.Fatal(err)
	}
	dockerEndpoint := "unix:///var/run/docker.sock"
	if platform.OS == "windows" {
		dockerEndpoint = "npipe:////./pipe/docker_engine"
	}
	profileLayer, receiverNames, err := collectorruntime.DockerMonitoringLayer(collectorMonitoringProfiles(registry.Profiles()), platform.OS, dockerEndpoint, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	dependencyCatalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	receiverLayer, err := dependencyMetricsReceiverLayer(dependencyCatalog, platform.OS, agentstate.State{}, receiverNames...)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := config.Render(config.RenderInput{
		Platform: platform, Layers: layers, InlineLayers: []config.InlineLayer{receiverLayer, profileLayer},
		Values: map[string]string{"OTEL_GATEWAY_ENDPOINT": "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "native-profiles.yaml")
	if err := os.WriteFile(configPath, rendered, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), binary, "validate", "--config", configPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pinned %s Collector rejected local PostgreSQL/Prometheus profiles: %v\n%s\n%s", platform.OS, err, output, rendered)
	}
}

func TestNativeMonitoringInputsAcceptPinnedPostgreSQLProfileCapability(t *testing.T) {
	t.Setenv("AR_IMMS_PG_USER", "metrics_reader")
	t.Setenv("AR_IMMS_PG_PASSWORD", "test-only-secret")
	statePath := filepath.Join(t.TempDir(), "state.db")
	installationID, err := installationapp.EnsureInstallationIdentity(sqlitefixture.NewIdentityStore(statePath), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seedRegisteredEnrollment(t, statePath, installationID.String(), "33333333-4444-4555-8666-777777777777")
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatal(err)
	}
	dockerEndpoint := "unix:///var/run/docker.sock"
	if platform.OS == "windows" {
		dockerEndpoint = "npipe:////./pipe/docker_engine"
	}
	t.Setenv(dockerDiscoveryEndpointEnv, dockerEndpoint)
	postgres := monitoringdomain.Profile{
		ID: "postgres", Provider: "docker", MatchLabels: map[string]string{monitoringdomain.LabelMonitoringEnabled: "true", monitoringdomain.LabelMonitoringProfile: "postgres"},
		RequiredCapabilities: []string{monitoringdomain.PostgresqlCapability}, ReceiverType: "postgresql", CollectionInterval: "10s",
		EndpointStrategies: []monitoringdomain.EndpointStrategy{{ID: "alternate_port", OS: platform.OS, When: "alternate_port > 0 and port == 5432", Endpoint: "127.0.0.1:`alternate_port`", Scope: monitoringdomain.EndpointHost, Provenance: "published_port", TargetPort: 5432, Priority: 100}},
		Settings:           monitoringdomain.ReceiverSettings{UsernameEnv: "AR_IMMS_PG_USER", PasswordEnv: "AR_IMMS_PG_PASSWORD"},
	}
	registry, err := monitoringdomain.NewRegistry(postgres)
	if err != nil {
		t.Fatal(err)
	}
	if layers, receivers, err := nativeDockerMonitoringInputsWithProfiles(statePath, platform.OS, registry); err != nil || len(layers) != 1 || len(receivers) != 1 {
		t.Fatalf("native profile capability validation layers=%d receivers=%v error=%v", len(layers), receivers, err)
	}
}

type nativeOwnershipInventory struct {
	snapshot inventoryctx.InventorySnapshot
	err      error
}

func (i nativeOwnershipInventory) Snapshot() (inventoryctx.InventorySnapshot, error) {
	return i.snapshot, i.err
}

type nativeOwnershipDelegate struct {
	calls        int
	contribution definitionsdomain.Contribution
}

func (d *nativeOwnershipDelegate) ResolveWithContribution(instance desiredstatedomain.Instance) (desiredstatedomain.Instance, definitionsdomain.Contribution, error) {
	d.calls++
	return instance, d.contribution, nil
}

func TestNativeOwnershipResolverDelegatesExactlyOneReceiverOwner(t *testing.T) {
	intent, err := definitionsdomain.NewComponentIntent("redis", map[string]any{"endpoint": "127.0.0.1:16379"})
	if err != nil {
		t.Fatal(err)
	}
	delegate := &nativeOwnershipDelegate{contribution: definitionsdomain.NewContribution([]definitionsdomain.ComponentIntent{intent})}
	resource := inventoryctx.Resource{
		Key: inventoryctx.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Attributes: map[string]string{
			"label.ar-imms.monitoring.enabled": "true", "label.ar-imms.monitoring.profile": "redis",
		},
	}
	resolver := nativeOwnershipResolver{
		delegate: delegate, inventory: nativeOwnershipInventory{snapshot: inventoryctx.InventorySnapshot{Resources: []inventoryctx.Resource{resource}}},
		profiles: monitoringdomain.DefaultRegistry(), enabled: true,
	}
	resolved, err := resolver.Resolve(nativeBoundRedisInstance("container-a"))
	if err != nil {
		t.Fatal(err)
	}
	if delegate.calls != 1 || len(resolved.Intents()) != 0 {
		t.Fatalf("native target receiver ownership calls=%d contribution=%v; want one validation and no Agent receiver", delegate.calls, resolved.Intents())
	}
}

func TestNativeOwnershipResolverKeepsLegacyAndUnmatchedTargetsAgentOwned(t *testing.T) {
	intent, err := definitionsdomain.NewComponentIntent("redis", map[string]any{"endpoint": "127.0.0.1:16379"})
	if err != nil {
		t.Fatal(err)
	}
	for name, resource := range map[string]inventoryctx.Resource{
		"unlabelled":       {Key: inventoryctx.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"}},
		"different target": {Key: inventoryctx.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-b"}, Attributes: map[string]string{"label.ar-imms.monitoring.enabled": "true", "label.ar-imms.monitoring.profile": "redis"}},
	} {
		t.Run(name, func(t *testing.T) {
			delegate := &nativeOwnershipDelegate{contribution: definitionsdomain.NewContribution([]definitionsdomain.ComponentIntent{intent})}
			resolver := nativeOwnershipResolver{
				delegate: delegate, inventory: nativeOwnershipInventory{snapshot: inventoryctx.InventorySnapshot{Resources: []inventoryctx.Resource{resource}}},
				profiles: monitoringdomain.DefaultRegistry(), enabled: true,
			}
			resolved, err := resolver.Resolve(nativeBoundRedisInstance("container-a"))
			if err != nil {
				t.Fatal(err)
			}
			if delegate.calls != 1 || len(resolved.Intents()) != 1 {
				t.Fatalf("Agent-owned target contribution=%v calls=%d", resolved.Intents(), delegate.calls)
			}
		})
	}
}

func TestNativeOwnershipResolverFailsClosedWhenInventoryCannotBeRead(t *testing.T) {
	delegate := &nativeOwnershipDelegate{}
	resolver := nativeOwnershipResolver{
		delegate: delegate, inventory: nativeOwnershipInventory{err: errors.New("inventory unavailable")},
		profiles: monitoringdomain.DefaultRegistry(), enabled: true,
	}
	if _, err := resolver.Resolve(nativeBoundRedisInstance("container-a")); err == nil {
		t.Fatal("Resolve() succeeded without ownership evidence")
	}
	if delegate.calls != 0 {
		t.Fatalf("delegate called %d times after ownership evidence failed", delegate.calls)
	}
}

func nativeBoundRedisInstance(resourceID string) desiredstatedomain.Instance {
	return desiredstatedomain.Instance{
		ID: "redis-instance", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:16379"}, Enabled: true,
		Binding: &desiredstatedomain.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: resourceID},
	}
}

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
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
	profileLayer, receiverNames, err := monitoringprofile.DefaultRegistry().DockerCollectorLayer(platform.OS, dockerEndpoint, "11111111-2222-4333-8444-555555555555")
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
	registry, err := monitoringprofile.LoadDirectory(root)
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
	profileLayer, receiverNames, err := registry.DockerCollectorLayer(platform.OS, dockerEndpoint, "11111111-2222-4333-8444-555555555555")
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
	installationID, err := identity.EnsureInstallationIdentity(agentstate.NewIdentityStore(statePath), rand.Reader)
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
	postgres := monitoringprofile.Profile{
		ID: "postgres", Provider: "docker", MatchLabels: map[string]string{monitoringprofile.LabelMonitoringEnabled: "true", monitoringprofile.LabelMonitoringProfile: "postgres"},
		RequiredCapabilities: []string{monitoringprofile.PostgresqlCapability}, ReceiverType: "postgresql", CollectionInterval: "10s",
		EndpointStrategies: []monitoringprofile.EndpointStrategy{{ID: "alternate_port", OS: platform.OS, When: "alternate_port > 0 and port == 5432", Endpoint: "127.0.0.1:`alternate_port`", Scope: discovery.EndpointHost, Provenance: "published_port", TargetPort: 5432, Priority: 100}},
		Settings:           monitoringprofile.ReceiverSettings{UsernameEnv: "AR_IMMS_PG_USER", PasswordEnv: "AR_IMMS_PG_PASSWORD"},
	}
	registry, err := monitoringprofile.NewRegistry(postgres)
	if err != nil {
		t.Fatal(err)
	}
	if layers, receivers, err := nativeDockerMonitoringInputsWithProfiles(statePath, platform.OS, registry); err != nil || len(layers) != 1 || len(receivers) != 1 {
		t.Fatalf("native profile capability validation layers=%d receivers=%v error=%v", len(layers), receivers, err)
	}
}

type nativeOwnershipInventory struct {
	snapshot discovery.InventorySnapshot
	err      error
}

func (i nativeOwnershipInventory) Snapshot() (discovery.InventorySnapshot, error) {
	return i.snapshot, i.err
}

type nativeOwnershipDelegate struct {
	calls        int
	contribution integration.Contribution
}

func (d *nativeOwnershipDelegate) Resolve(instance integration.IntegrationInstance) (integration.ResolvedInstance, error) {
	d.calls++
	return integration.ResolvedInstance{Instance: instance, Contribution: d.contribution}, nil
}

func TestNativeOwnershipResolverDelegatesExactlyOneReceiverOwner(t *testing.T) {
	intent, err := integration.NewComponentIntent("redis", map[string]any{"endpoint": "127.0.0.1:16379"})
	if err != nil {
		t.Fatal(err)
	}
	delegate := &nativeOwnershipDelegate{contribution: integration.NewContribution([]integration.ComponentIntent{intent})}
	resource := discovery.Resource{
		Key: discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Attributes: map[string]string{
			"label.ar-imms.monitoring.enabled": "true", "label.ar-imms.monitoring.profile": "redis",
		},
	}
	resolver := nativeOwnershipResolver{
		delegate: delegate, inventory: nativeOwnershipInventory{snapshot: discovery.InventorySnapshot{Resources: []discovery.Resource{resource}}},
		profiles: monitoringprofile.DefaultRegistry(), enabled: true,
	}
	resolved, err := resolver.Resolve(nativeBoundRedisInstance("container-a"))
	if err != nil {
		t.Fatal(err)
	}
	if delegate.calls != 1 || len(resolved.Contribution.Intents()) != 0 {
		t.Fatalf("native target receiver ownership calls=%d contribution=%v; want one validation and no Agent receiver", delegate.calls, resolved.Contribution.Intents())
	}
}

func TestNativeOwnershipResolverKeepsLegacyAndUnmatchedTargetsAgentOwned(t *testing.T) {
	intent, err := integration.NewComponentIntent("redis", map[string]any{"endpoint": "127.0.0.1:16379"})
	if err != nil {
		t.Fatal(err)
	}
	for name, resource := range map[string]discovery.Resource{
		"unlabelled":       {Key: discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"}},
		"different target": {Key: discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-b"}, Attributes: map[string]string{"label.ar-imms.monitoring.enabled": "true", "label.ar-imms.monitoring.profile": "redis"}},
	} {
		t.Run(name, func(t *testing.T) {
			delegate := &nativeOwnershipDelegate{contribution: integration.NewContribution([]integration.ComponentIntent{intent})}
			resolver := nativeOwnershipResolver{
				delegate: delegate, inventory: nativeOwnershipInventory{snapshot: discovery.InventorySnapshot{Resources: []discovery.Resource{resource}}},
				profiles: monitoringprofile.DefaultRegistry(), enabled: true,
			}
			resolved, err := resolver.Resolve(nativeBoundRedisInstance("container-a"))
			if err != nil {
				t.Fatal(err)
			}
			if delegate.calls != 1 || len(resolved.Contribution.Intents()) != 1 {
				t.Fatalf("Agent-owned target contribution=%v calls=%d", resolved.Contribution.Intents(), delegate.calls)
			}
		})
	}
}

func TestNativeOwnershipResolverFailsClosedWhenInventoryCannotBeRead(t *testing.T) {
	delegate := &nativeOwnershipDelegate{}
	resolver := nativeOwnershipResolver{
		delegate: delegate, inventory: nativeOwnershipInventory{err: errors.New("inventory unavailable")},
		profiles: monitoringprofile.DefaultRegistry(), enabled: true,
	}
	if _, err := resolver.Resolve(nativeBoundRedisInstance("container-a")); err == nil {
		t.Fatal("Resolve() succeeded without ownership evidence")
	}
	if delegate.calls != 0 {
		t.Fatalf("delegate called %d times after ownership evidence failed", delegate.calls)
	}
}

func nativeBoundRedisInstance(resourceID string) integration.IntegrationInstance {
	return integration.IntegrationInstance{
		ID: "redis-instance", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:16379"}, Enabled: true,
		Binding: &integration.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: resourceID},
	}
}

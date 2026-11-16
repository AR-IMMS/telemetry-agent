package yamlprofiles_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	monitoringprofile "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles"
	profilesapplication "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/application"
	yamlprofiles "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/yaml"
	collectorruntime "github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
)

func TestLoadDirectoryAddsValidatedPrometheusProfiles(t *testing.T) {
	root := t.TempDir()
	writeProfileFile(t, root, "cooling.yaml", `
api_version: ar-imms/v1alpha1
profiles:
  - id: cooling-exporter
    provider: docker
    receiver_type: prometheus
    required_capabilities: [otel.receiver.prometheus.metrics]
    match_labels:
      ar-imms.monitoring.enabled: "true"
      ar-imms.monitoring.profile: cooling-exporter
    collection_interval: 15s
    target_port: 9200
    target_port_label: ar-imms.monitoring.port
    metrics_path: /metrics
`)

	registry, err := profilesapplication.LoadDirectory(root, yamlprofiles.DirectorySource{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Match("docker", map[string]string{
		"label." + monitoringprofile.LabelMonitoringEnabled: "true",
		"label." + monitoringprofile.LabelMonitoringProfile: "cooling-exporter",
	}); !ok {
		t.Fatal("loaded profile did not match opt-in labels")
	}
	layer, _, err := collectorruntime.DockerMonitoringLayer(runtimeProfileInputs(registry.Profiles()), "linux", dockerEndpoint("linux"), "core-node")
	if err != nil {
		t.Fatal(err)
	}
	creator := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)
	raw := creator["receivers"].(map[string]any)["prometheus/cooling-exporter_alternate_port"]
	if raw == nil {
		t.Fatal("data-defined Prometheus receiver rule was not generated")
	}
	rule := raw.(map[string]any)["rule"].(string)
	if !strings.Contains(rule, "port == 9200") || !strings.Contains(rule, `labels["ar-imms.monitoring.profile"] == "cooling-exporter"`) {
		t.Fatalf("generated rule=%s", rule)
	}
	config := raw.(map[string]any)["config"].(map[string]any)
	scrape := config["config"].(map[string]any)["scrape_configs"].([]any)[0].(map[string]any)
	if scrape["metrics_path"] != "/metrics" || scrape["job_name"] != "cooling-exporter" {
		t.Fatalf("generated scrape config=%#v", scrape)
	}
}

func TestLoadDirectoryRejectsUnknownFieldsAndUnsafeReceiverConfig(t *testing.T) {
	for name, content := range map[string]string{
		"unknown-field": `api_version: ar-imms/v1alpha1
profiles:
  - id: custom
    provider: docker
    receiver_type: prometheus
    required_capabilities: [otel.receiver.prometheus.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: custom}
    collection_interval: 15s
    target_port: 9100
    metrics_path: /metrics
    raw_collector_config: {receivers: {otlp: {}}}
`,
		"unsafe-path": `api_version: ar-imms/v1alpha1
profiles:
  - id: custom
    provider: docker
    receiver_type: prometheus
    required_capabilities: [otel.receiver.prometheus.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: custom}
    collection_interval: 15s
    target_port: 9100
    metrics_path: /metrics?x=1
`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeProfileFile(t, root, "profile.yaml", content)
			if _, err := profilesapplication.LoadDirectory(root, yamlprofiles.DirectorySource{}); err == nil {
				t.Fatal("LoadDirectory accepted invalid profile")
			}
		})
	}
}

func TestLoadDirectoryRequiresPostgreSQLCredentialReferences(t *testing.T) {
	t.Setenv("AR_IMMS_PG_USER", "metrics_reader")
	t.Setenv("AR_IMMS_PG_PASSWORD", "test-only-secret")
	root := t.TempDir()
	writeProfileFile(t, root, "postgres.yaml", `
api_version: ar-imms/v1alpha1
profiles:
  - id: postgres
    provider: docker
    receiver_type: postgresql
    required_capabilities: [otel.receiver.postgresql.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: postgres}
    collection_interval: 30s
    target_port: 5432
    target_port_label: ar-imms.monitoring.port
    username_env: AR_IMMS_PG_USER
    password_env: AR_IMMS_PG_PASSWORD
`)
	registry, err := profilesapplication.LoadDirectory(root, yamlprofiles.DirectorySource{})
	if err != nil {
		t.Fatal(err)
	}
	layer, _, err := collectorruntime.DockerMonitoringLayer(runtimeProfileInputs(registry.Profiles()), "linux", dockerEndpoint("linux"), "core-node")
	if err != nil {
		t.Fatal(err)
	}
	creator := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)
	receivers := creator["receivers"].(map[string]any)
	postgres := receivers["postgresql/postgres_alternate_port"].(map[string]any)
	config := postgres["config"].(map[string]any)
	if config["username"] != "${env:AR_IMMS_PG_USER}" || config["password"] != "${env:AR_IMMS_PG_PASSWORD}" {
		t.Fatalf("credential references must be environment references, got %#v", config)
	}
}

func TestPostgreSQLProfileRequiresCredentialsBeforeReceiverProjection(t *testing.T) {
	t.Setenv("AR_IMMS_PG_USER", "")
	t.Setenv("AR_IMMS_PG_PASSWORD", "")
	root := t.TempDir()
	writeProfileFile(t, root, "postgres.yaml", `
api_version: ar-imms/v1alpha1
profiles:
  - id: postgres
    provider: docker
    receiver_type: postgresql
    required_capabilities: [otel.receiver.postgresql.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: postgres}
    collection_interval: 30s
    target_port: 5432
    target_port_label: ar-imms.monitoring.port
    username_env: AR_IMMS_PG_USER
    password_env: AR_IMMS_PG_PASSWORD
`)
	registry, err := profilesapplication.LoadDirectory(root, yamlprofiles.DirectorySource{})
	if err != nil {
		t.Fatal(err)
	}
	profile, matched := registry.Match("docker", map[string]string{
		"label." + monitoringprofile.LabelMonitoringEnabled: "true",
		"label." + monitoringprofile.LabelMonitoringProfile: "postgres",
	})
	if !matched || collectorruntime.ProfileCredentialsConfigured(profile.ReceiverType, profile.Settings.UsernameEnv, profile.Settings.PasswordEnv) {
		t.Fatal("PostgreSQL profile should match while reporting unavailable credentials")
	}
	layer, _, err := collectorruntime.DockerMonitoringLayer(runtimeProfileInputs(registry.Profiles()), "linux", dockerEndpoint("linux"), "core-node")
	if err != nil {
		t.Fatal(err)
	}
	creator := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)
	if _, exists := creator["receivers"].(map[string]any)["postgresql/postgres_alternate_port"]; exists {
		t.Fatal("Receiver Creator rule was generated without PostgreSQL credentials")
	}
}

func TestLoadDirectoryRejectsMissingPostgreSQLCredentialReferences(t *testing.T) {
	root := t.TempDir()
	writeProfileFile(t, root, "postgres.yaml", `
api_version: ar-imms/v1alpha1
profiles:
  - id: postgres
    provider: docker
    receiver_type: postgresql
    required_capabilities: [otel.receiver.postgresql.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: postgres}
    collection_interval: 30s
    target_port: 5432
`)
	if _, err := profilesapplication.LoadDirectory(root, yamlprofiles.DirectorySource{}); err == nil {
		t.Fatal("LoadDirectory accepted PostgreSQL profile without credential references")
	}
}

func dockerEndpoint(os string) string {
	if os == "windows" {
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}

func runtimeProfileInputs(profiles []monitoringprofile.Profile) []contracts.DockerMonitoringProfile {
	result := make([]contracts.DockerMonitoringProfile, 0, len(profiles))
	for _, profile := range profiles {
		converted := contracts.DockerMonitoringProfile{
			ID: profile.ID, Provider: profile.Provider, MatchLabels: profile.MatchLabels,
			RequiredCapabilities: profile.RequiredCapabilities, ReceiverType: profile.ReceiverType,
			CollectionInterval: profile.CollectionInterval,
			Settings:           contracts.DockerReceiverSettings{MetricsPath: profile.Settings.MetricsPath, UsernameEnv: profile.Settings.UsernameEnv, PasswordEnv: profile.Settings.PasswordEnv, TLSCAFile: profile.Settings.TLSCAFile},
		}
		for _, strategy := range profile.EndpointStrategies {
			converted.EndpointStrategies = append(converted.EndpointStrategies, contracts.DockerEndpointStrategy{
				ID: strategy.ID, OS: strategy.OS, When: strategy.When, Endpoint: strategy.Endpoint,
				Scope: string(strategy.Scope), Provenance: strategy.Provenance, TargetPort: strategy.TargetPort,
				TargetPortLabel: strategy.TargetPortLabel, PrivatePortVariable: strategy.PrivatePortVariable, Priority: strategy.Priority,
			})
		}
		result = append(result, converted)
	}
	return result
}

func writeProfileFile(t *testing.T, root, name, content string) {
	t.Helper()
	directory := filepath.Join(root, "monitoring-profiles")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

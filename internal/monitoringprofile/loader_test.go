package monitoringprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	registry, err := LoadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Match("docker", map[string]string{
		"label." + LabelMonitoringEnabled: "true",
		"label." + LabelMonitoringProfile: "cooling-exporter",
	}); !ok {
		t.Fatal("loaded profile did not match opt-in labels")
	}
	layer, _, err := registry.DockerCollectorLayer("linux", dockerEndpoint("linux"), "core-node")
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
			if _, err := LoadDirectory(root); err == nil {
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
	registry, err := LoadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	layer, _, err := registry.DockerCollectorLayer("linux", dockerEndpoint("linux"), "core-node")
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
	registry, err := LoadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	profile, matched := registry.Match("docker", map[string]string{
		"label." + LabelMonitoringEnabled: "true",
		"label." + LabelMonitoringProfile: "postgres",
	})
	if !matched || CredentialsConfigured(profile) {
		t.Fatal("PostgreSQL profile should match while reporting unavailable credentials")
	}
	layer, _, err := registry.DockerCollectorLayer("linux", dockerEndpoint("linux"), "core-node")
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
	if _, err := LoadDirectory(root); err == nil {
		t.Fatal("LoadDirectory accepted PostgreSQL profile without credential references")
	}
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

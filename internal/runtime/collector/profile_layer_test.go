package collectorruntime

import (
	"strings"
	"testing"

	monitoringprofile "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
)

func TestDockerMonitoringLayerProjectsRegisteredProfiles(t *testing.T) {
	registry, err := monitoringprofile.NewRegistry(monitoringprofile.RedisProfile())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		os                string
		endpoint          string
		wantContainerRule bool
	}{
		{os: "windows", endpoint: "npipe:////./pipe/docker_engine"},
		{os: "linux", endpoint: "unix:///var/run/docker.sock", wantContainerRule: true},
	} {
		t.Run(test.os, func(t *testing.T) {
			layer, receivers, err := DockerMonitoringLayer(runtimeProfileInputs(registry.Profiles()), test.os, test.endpoint, "core-node-123")
			if err != nil {
				t.Fatal(err)
			}
			if len(receivers) != 1 || receivers[0] != "receiver_creator" {
				t.Fatalf("receivers=%v", receivers)
			}
			extensions := layer.Document["extensions"].(map[string]any)
			observer := extensions["docker_observer"].(map[string]any)
			if observer["endpoint"] != test.endpoint {
				t.Fatalf("observer endpoint=%v", observer["endpoint"])
			}
			creator := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)
			configured := creator["receivers"].(map[string]any)
			if configured["redis/alternate_port"] == nil {
				t.Fatal("alternate-port Redis rule is missing")
			}
			_, hasContainer := configured["redis/container_endpoint"]
			if hasContainer != test.wantContainerRule {
				t.Fatalf("container endpoint rule present=%t", hasContainer)
			}
			alternate := configured["redis/alternate_port"].(map[string]any)
			rule := alternate["rule"].(string)
			if !strings.Contains(rule, `labels["ar-imms.monitoring.enabled"] == "true"`) || !strings.Contains(rule, `labels["ar-imms.monitoring.profile"] == "redis"`) || !strings.Contains(rule, "6379") {
				t.Fatalf("Redis rule=%s", rule)
			}
			attributes := alternate["resource_attributes"].(map[string]any)
			if attributes["node_id"] != "core-node-123" || attributes["resource_id"] != "`container_id`" || attributes["workload_id"] != "`labels[\"ar-imms.workload.id\"]`" {
				t.Fatalf("resource identity attributes=%#v", attributes)
			}
			if test.os == "linux" && (observer["use_host_bindings"] != true || observer["ignore_non_host_bindings"] != true) {
				t.Fatalf("Linux endpoint selection=%#v", observer)
			}
		})
	}
}

func TestDockerMonitoringLayerProjectsExtensibleReceiverProfiles(t *testing.T) {
	t.Setenv("AR_IMMS_PG_USER", "metrics_reader")
	t.Setenv("AR_IMMS_PG_PASSWORD", "test-only-secret")
	redis := monitoringprofile.RedisProfile()
	postgres := monitoringprofile.Profile{
		ID: "postgres", Provider: "docker", MatchLabels: map[string]string{monitoringprofile.LabelMonitoringEnabled: "true", monitoringprofile.LabelMonitoringProfile: "postgres"},
		RequiredCapabilities: []string{monitoringprofile.PostgresqlCapability}, ReceiverType: "postgresql", CollectionInterval: "10s",
		EndpointStrategies: []monitoringprofile.EndpointStrategy{{ID: "alternate_port", OS: "windows", When: "alternate_port > 0", Endpoint: "127.0.0.1:`alternate_port`", Scope: "host", Provenance: "published_port", Priority: 100}},
		Settings:           monitoringprofile.ReceiverSettings{UsernameEnv: "AR_IMMS_PG_USER", PasswordEnv: "AR_IMMS_PG_PASSWORD"},
	}
	registry, err := monitoringprofile.NewRegistry(redis, postgres)
	if err != nil {
		t.Fatal(err)
	}
	layer, receivers, err := DockerMonitoringLayer(runtimeProfileInputs(registry.Profiles()), "windows", "npipe:////./pipe/docker_engine", "core-node-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(receivers) != 1 || receivers[0] != "receiver_creator" {
		t.Fatalf("receiver pipeline entries=%v", receivers)
	}
	configured := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)["receivers"].(map[string]any)
	if configured["redis/alternate_port"] == nil || configured["postgresql/alternate_port"] == nil {
		t.Fatalf("profiles were not both projected: %#v", configured)
	}
}

func TestProfileCredentialsConfiguredNeverNeedsCredentialValues(t *testing.T) {
	profile := monitoringprofile.Profile{ReceiverType: "postgresql", Settings: monitoringprofile.ReceiverSettings{UsernameEnv: "AR_IMMS_PG_USER", PasswordEnv: "AR_IMMS_PG_PASSWORD"}}
	t.Setenv("AR_IMMS_PG_USER", "")
	t.Setenv("AR_IMMS_PG_PASSWORD", "")
	if ProfileCredentialsConfigured(profile.ReceiverType, profile.Settings.UsernameEnv, profile.Settings.PasswordEnv) {
		t.Fatal("empty credential environment values were treated as configured")
	}
	t.Setenv("AR_IMMS_PG_USER", "metrics_reader")
	t.Setenv("AR_IMMS_PG_PASSWORD", "test-only-secret")
	if !ProfileCredentialsConfigured(profile.ReceiverType, profile.Settings.UsernameEnv, profile.Settings.PasswordEnv) {
		t.Fatal("configured credential environment values were not recognized")
	}
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

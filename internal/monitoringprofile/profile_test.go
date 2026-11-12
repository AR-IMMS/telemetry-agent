package monitoringprofile

import (
	"strings"
	"testing"
)

func TestRegistryRejectsDuplicateProfileIdentity(t *testing.T) {
	profile := RedisProfile()
	if _, err := NewRegistry(profile, profile); err == nil {
		t.Fatal("NewRegistry accepted duplicate profile identity")
	}
}

func TestRegistryRejectsConflictingReceiverRuleKeys(t *testing.T) {
	redis := RedisProfile()
	postgres := Profile{
		ID: "postgres", Provider: "docker", MatchLabels: map[string]string{LabelMonitoringEnabled: "true", LabelMonitoringProfile: "postgres"},
		RequiredCapabilities: []string{"otel.receiver.postgresql.metrics"}, ReceiverType: redis.ReceiverType, CollectionInterval: "10s",
		EndpointStrategies: []EndpointStrategy{{ID: "alternate_port", OS: "windows", When: "alternate_port > 0", Endpoint: "127.0.0.1:`alternate_port`", Scope: redis.EndpointStrategies[0].Scope, Provenance: "published_port", Priority: 100}},
	}
	if _, err := NewRegistry(redis, postgres); err == nil {
		t.Fatal("NewRegistry accepted profiles that would overwrite the same Receiver Creator rule")
	}
}

func TestRedisProfileRequiresDockerOptInLabels(t *testing.T) {
	registry, err := NewRegistry(RedisProfile())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		provider   string
		attributes map[string]string
		want       bool
	}{
		{name: "opted in", provider: "docker", attributes: map[string]string{"label." + LabelMonitoringEnabled: "true", "label." + LabelMonitoringProfile: "redis"}, want: true},
		{name: "disabled", provider: "docker", attributes: map[string]string{"label." + LabelMonitoringEnabled: "false", "label." + LabelMonitoringProfile: "redis"}},
		{name: "wrong profile", provider: "docker", attributes: map[string]string{"label." + LabelMonitoringEnabled: "true", "label." + LabelMonitoringProfile: "postgres"}},
		{name: "wrong provider", provider: "process", attributes: map[string]string{"label." + LabelMonitoringEnabled: "true", "label." + LabelMonitoringProfile: "redis"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, found := registry.Match(test.provider, test.attributes)
			if found != test.want {
				t.Fatalf("Match() found=%t, want %t", found, test.want)
			}
		})
	}
}

func TestDockerCollectorLayerUsesPlatformEndpointStrategiesAndIdentity(t *testing.T) {
	registry, err := NewRegistry(RedisProfile())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		os                string
		wantAlternate     bool
		wantContainer     bool
		wantReceiverNames []string
	}{
		{os: "windows", wantAlternate: true, wantReceiverNames: []string{"receiver_creator"}},
		{os: "linux", wantAlternate: true, wantContainer: true, wantReceiverNames: []string{"receiver_creator"}},
	} {
		t.Run(test.os, func(t *testing.T) {
			layer, receiverNames, err := registry.DockerCollectorLayer(test.os, dockerEndpoint(test.os), "core-node-123")
			if err != nil {
				t.Fatal(err)
			}
			if len(receiverNames) != len(test.wantReceiverNames) || receiverNames[0] != test.wantReceiverNames[0] {
				t.Fatalf("receiver names = %v, want %v", receiverNames, test.wantReceiverNames)
			}
			extensions := layer.Document["extensions"].(map[string]any)
			if extensions["docker_observer"].(map[string]any)["endpoint"] != dockerEndpoint(test.os) {
				t.Fatalf("Docker Observer endpoint = %#v", extensions["docker_observer"])
			}
			creator := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)
			receivers := creator["receivers"].(map[string]any)
			if test.wantAlternate && receivers["redis/alternate_port"] == nil {
				t.Fatal("alternate-port Redis receiver rule is missing")
			}
			if test.wantContainer && receivers["redis/container_endpoint"] == nil {
				t.Fatal("container-endpoint Redis receiver rule is missing")
			}
			for _, raw := range receivers {
				receiver := raw.(map[string]any)
				rule := receiver["rule"].(string)
				if !strings.Contains(rule, "port == 6379") {
					t.Fatalf("Redis receiver rule accepts a non-Redis private port: %s", rule)
				}
				if !containsAll(rule, `labels["ar-imms.monitoring.enabled"] == "true"`, `labels["ar-imms.monitoring.profile"] == "redis"`) {
					t.Fatalf("receiver rule lacks Redis opt-in labels: %s", rule)
				}
				attributes := receiver["resource_attributes"].(map[string]any)
				for _, key := range []string{"node_id", "resource_id", "container.name", "container.id"} {
					if attributes[key] == nil {
						t.Errorf("receiver resource attributes missing %q: %#v", key, attributes)
					}
				}
				if attributes["node_id"] != "core-node-123" || attributes["resource_id"] != "`container_id`" {
					t.Errorf("receiver identity attributes = %#v", attributes)
				}
				if attributes["workload_id"] != "`labels[\"ar-imms.workload.id\"]`" {
					t.Errorf("optional workload identity mapping = %#v", attributes["workload_id"])
				}
			}
		})
	}
}

func dockerEndpoint(os string) string {
	if os == "windows" {
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

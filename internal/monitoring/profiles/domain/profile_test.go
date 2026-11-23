package domain

import (
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

func TestRegistryRejectsInvalidTargetPortLabelContract(t *testing.T) {
	profile := RedisProfile()
	profile.EndpointStrategies[0].TargetPortLabel = `bad label`
	if _, err := NewRegistry(profile); err == nil {
		t.Fatal("NewRegistry accepted an invalid target-port label key")
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

package redis

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestRedisDefinitionMatchesExactImageAndBuildsLoopbackCandidate(t *testing.T) {
	definition := Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resource := redisMatchResource("docker.io/library/redis", integration.MatchEndpoint{
		Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port",
	})
	report, err := integration.NewMatcher(registry, nil).Match(integration.MatchSnapshot{
		InventoryRevision: 5, AsOf: resource.LastSeenAt, StaleAfter: time.Minute,
		Resources: []integration.MatchResource{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results=%+v", report.Results)
	}
	result := report.Results[0]
	if result.MatchStatus != integration.MatchMatched || result.ResolutionStatus != integration.ResolutionReady || result.Candidate == nil {
		t.Fatalf("result=%+v", result)
	}
	var configuration map[string]any
	if err := json.Unmarshal(result.Candidate.Configuration(), &configuration); err != nil {
		t.Fatal(err)
	}
	if configuration["endpoint"] != "127.0.0.1:6380" {
		t.Fatalf("candidate configuration=%v", configuration)
	}
	if result.CapabilityStatus != integration.CapabilityUnverified {
		t.Fatalf("missing catalog evidence capability status=%q", result.CapabilityStatus)
	}
}

func TestRedisDefinitionRequiresInputForUnsafeOrAmbiguousEndpoints(t *testing.T) {
	cases := []struct {
		name      string
		endpoints []integration.MatchEndpoint
	}{
		{name: "wildcard", endpoints: []integration.MatchEndpoint{{Address: "0.0.0.0", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port"}}},
		{name: "container private", endpoints: []integration.MatchEndpoint{{Address: "172.18.0.2", Port: 6379, Protocol: "tcp", Scope: "container", Provenance: "container_network"}}},
		{name: "exposed only", endpoints: []integration.MatchEndpoint{{Port: 6379, Protocol: "tcp", Scope: "container", Provenance: "exposed_port"}}},
		{name: "remote host", endpoints: []integration.MatchEndpoint{{Address: "192.168.1.10", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port"}}},
		{name: "ambiguous", endpoints: []integration.MatchEndpoint{
			{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port"},
			{Address: "127.0.0.1", Port: 6381, Protocol: "tcp", Scope: "host", Provenance: "published_port"},
		}},
	}
	definition := Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource := redisMatchResource("docker.io/library/redis", tc.endpoints...)
			report, err := integration.NewMatcher(registry, nil).Match(integration.MatchSnapshot{
				AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []integration.MatchResource{resource},
			})
			if err != nil {
				t.Fatal(err)
			}
			result := report.Results[0]
			if result.MatchStatus != integration.MatchMatched || result.ResolutionStatus != integration.ResolutionNeedsInput || result.Candidate != nil || len(result.RequiredInputs) != 1 {
				t.Fatalf("unsafe endpoint became actionable: %+v", result)
			}
		})
	}
}

func TestRedisDefinitionUsesExactImageRepository(t *testing.T) {
	definition := Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resource := redisMatchResource("registry.example/team/not-redis", integration.MatchEndpoint{
		Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port",
	})
	report, err := integration.NewMatcher(registry, nil).Match(integration.MatchSnapshot{
		AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []integration.MatchResource{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].MatchStatus != integration.MatchNoMatch || report.Results[0].Candidate != nil {
		t.Fatalf("non-Redis image matched: %+v", report.Results)
	}
}

func TestRedisDefinitionFormatsIPv6LoopbackCandidate(t *testing.T) {
	definition := Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resource := redisMatchResource("docker.io/library/redis", integration.MatchEndpoint{
		Address: "::1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port",
	})
	report, err := integration.NewMatcher(registry, nil).Match(integration.MatchSnapshot{
		AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []integration.MatchResource{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	var configuration map[string]any
	if err := json.Unmarshal(report.Results[0].Candidate.Configuration(), &configuration); err != nil {
		t.Fatal(err)
	}
	if configuration["endpoint"] != "[::1]:6380" {
		t.Fatalf("IPv6 endpoint=%v", configuration["endpoint"])
	}
}

func redisMatchResource(repository string, endpoints ...integration.MatchEndpoint) integration.MatchResource {
	return integration.MatchResource{
		Provider: "docker", ProviderInstance: "local-engine", ResourceID: "redis-container",
		Kind: "container", LifecycleState: "running", Presence: "present",
		Attributes: map[string]string{"container.image.repository": repository},
		Endpoints:  endpoints, LastSeenAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}
}

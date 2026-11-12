package monitoringprofile

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

func TestDiagnoseEndpointDistinguishesMissingAmbiguousAndResolved(t *testing.T) {
	profile := RedisProfile()
	tests := []struct {
		name      string
		osName    string
		endpoints []discovery.Endpoint
		want      EndpointState
		count     int
	}{
		{name: "no host mapping on Windows", osName: "windows", endpoints: []discovery.Endpoint{{Address: "172.19.0.3", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}}, want: EndpointMissing},
		{name: "one published target", osName: "windows", endpoints: []discovery.Endpoint{{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port", ContainerPort: 6379}}, want: EndpointResolved, count: 1},
		{name: "two published targets", osName: "windows", endpoints: []discovery.Endpoint{{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port", ContainerPort: 6379}, {Address: "127.0.0.1", Port: 16380, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port", ContainerPort: 6379}}, want: EndpointAmbiguous, count: 2},
		{name: "host mapping wins over multiple network addresses", osName: "linux", endpoints: []discovery.Endpoint{{Address: "172.19.0.3", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}, {Address: "172.20.0.3", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}, {Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port", ContainerPort: 6379}}, want: EndpointResolved, count: 1},
		{name: "multiple unbound networks are ambiguous", osName: "linux", endpoints: []discovery.Endpoint{{Address: "172.19.0.3", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}, {Address: "172.20.0.3", Port: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}}, want: EndpointAmbiguous, count: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := DiagnoseEndpoint(profile, test.osName, test.endpoints, nil)
			if got.State != test.want || got.CandidateCount != test.count {
				t.Fatalf("diagnosis=%+v, want state=%q candidates=%d", got, test.want, test.count)
			}
		})
	}
}

func TestDiagnoseEndpointUsesValidatedPortOverrideAndDeterministicNetwork(t *testing.T) {
	profile := RedisProfile()
	attributes := map[string]string{"label." + LabelMonitoringPort: "6380"}
	endpoints := []discovery.Endpoint{
		{Address: "172.20.0.4", NetworkName: "zeta", NetworkPriority: 2, Port: 6380, ContainerPort: 6380, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
		{Address: "172.19.0.4", NetworkName: "alpha", NetworkPriority: 1, Port: 6380, ContainerPort: 6380, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
	}
	got := DiagnoseEndpointWithAttributes(profile, "linux", endpoints, nil, attributes)
	if got.State != EndpointResolved || got.CandidateCount != 2 || got.Endpoint.NetworkName != "zeta" || got.Endpoint.Address != "172.20.0.4" {
		t.Fatalf("deterministic port-override diagnosis=%+v, want highest-priority zeta network at port 6380", got)
	}
	if defaultPort := DiagnoseEndpoint(profile, "linux", endpoints, nil); defaultPort.State != EndpointMissing {
		t.Fatalf("default port diagnosis=%+v, want missing for port-6380-only workload", defaultPort)
	}
}

func TestDiagnoseEndpointRejectsInvalidPortOverride(t *testing.T) {
	for _, value := range []string{"0", "65536", "+6380", "6380x", " 6380", "１２３"} {
		t.Run(value, func(t *testing.T) {
			got := DiagnoseEndpointWithAttributes(RedisProfile(), "linux", nil, nil, map[string]string{"label." + LabelMonitoringPort: value})
			if got.State != EndpointInvalid || got.EvidenceCode != "invalid_target_port" {
				t.Fatalf("invalid target port diagnosis=%+v", got)
			}
		})
	}
}

func TestDiagnoseEndpointUsesNetworkNameForPriorityTie(t *testing.T) {
	profile := RedisProfile()
	endpoints := []discovery.Endpoint{
		{Address: "172.20.0.4", NetworkName: "zeta", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
		{Address: "172.19.0.4", NetworkName: "alpha", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
	}
	got := DiagnoseEndpoint(profile, "linux", endpoints, nil)
	if got.State != EndpointResolved || got.Endpoint.NetworkName != "alpha" {
		t.Fatalf("network priority tie diagnosis=%+v, want alphabetical network alpha", got)
	}
}

func TestDiagnoseEndpointFailsClosedWithoutNetworkIdentity(t *testing.T) {
	profile := RedisProfile()
	endpoints := []discovery.Endpoint{
		{Address: "172.20.0.4", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
		{Address: "172.19.0.4", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: discovery.EndpointContainer, Provenance: "container_network"},
	}
	got := DiagnoseEndpoint(profile, "linux", endpoints, nil)
	if got.State != EndpointAmbiguous || got.CandidateCount != 2 {
		t.Fatalf("missing network identity diagnosis=%+v, want fail-closed ambiguity", got)
	}
}

func TestDiagnoseEndpointRequiresMatchingOTelScrapeFailureForUnreachable(t *testing.T) {
	profile := RedisProfile()
	endpoint := discovery.Endpoint{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: discovery.EndpointHost, Provenance: "published_port", ContainerPort: 6379}
	withoutEvidence := DiagnoseEndpoint(profile, "windows", []discovery.Endpoint{endpoint}, nil)
	if withoutEvidence.State != EndpointResolved {
		t.Fatalf("discovered endpoint state=%q, want resolved but unverified", withoutEvidence.State)
	}
	evidence := []CollectorScrapeEvidence{{ReceiverID: "redis/alternate_port", Endpoint: "127.0.0.1:16379", FailureCode: "connection_failed"}}
	got := DiagnoseEndpoint(profile, "windows", []discovery.Endpoint{endpoint}, evidence)
	if got.State != EndpointUnreachable || got.EvidenceSource != EvidenceCollectorScrapeError {
		t.Fatalf("diagnosis=%+v, want OTel-evidenced unreachable endpoint", got)
	}
}

func TestParseCollectorScrapeFailureUsesOTelReceiverAndNetworkErrorEvidence(t *testing.T) {
	line := `2026-11-12T20:33:22.592+0700 error scraperhelper@v0.160.0/obs_metrics.go:61 Error scraping metrics {"otelcol.component.id":"redis/alternate_port","otelcol.component.kind":"receiver","otelcol.signal":"metrics","scraper":"redis","error":"dial tcp 127.0.0.1:16379: connectex: connection refused"}`
	evidence, ok := ParseCollectorLog(line)
	if !ok {
		t.Fatal("ParseCollectorLog() did not recognize OTel Redis scrape failure")
	}
	if evidence.ReceiverID != "redis/alternate_port" || evidence.Endpoint != "127.0.0.1:16379" || evidence.FailureCode != "connection_failed" {
		t.Fatalf("evidence=%+v", evidence)
	}
	if _, ok := ParseCollectorLog(`2026-11-12T20:33:22.592+0700 info service ready {"otelcol.component.id":"receiver_creator"}`); ok {
		t.Fatal("non-failure Collector log was treated as an endpoint failure")
	}
}

func TestParseCollectorReceiverCreatorFailureUsesDynamicReceiverName(t *testing.T) {
	line := `2026-11-12T20:33:22.592+0700 error scraperhelper@v0.160.0/obs_metrics.go:61 Error scraping metrics {"otelcol.component.id":"receiver_creator","otelcol.component.kind":"extension","otelcol.signal":"metrics","name":"redis/alternate_port/receiver_creator{endpoint=\"127.0.0.1:16379\"}/container-id:16379","scraper":"redis","error":"dial tcp 127.0.0.1:16379: connectex: connection refused"}`
	evidence, ok := ParseCollectorLog(line)
	if !ok {
		t.Fatal("ParseCollectorLog() did not recognize Receiver Creator dynamic receiver failure")
	}
	if evidence.ReceiverID != "redis/alternate_port" || evidence.Endpoint != "127.0.0.1:16379" || evidence.FailureCode != "connection_failed" {
		t.Fatalf("evidence=%+v", evidence)
	}
}

func TestRegistryExtendsDockerProfilesWithoutChangingDiscoveryProvider(t *testing.T) {
	t.Setenv("AR_IMMS_PG_USER", "metrics_reader")
	t.Setenv("AR_IMMS_PG_PASSWORD", "test-only-secret")
	redis := RedisProfile()
	postgres := Profile{
		ID: "postgres", Provider: "docker", MatchLabels: map[string]string{LabelMonitoringEnabled: "true", LabelMonitoringProfile: "postgres"},
		RequiredCapabilities: []string{"otel.receiver.postgresql.metrics"}, ReceiverType: "postgresql", CollectionInterval: "10s",
		EndpointStrategies: []EndpointStrategy{{ID: "alternate_port", OS: "windows", When: "alternate_port > 0", Endpoint: "127.0.0.1:`alternate_port`", Scope: discovery.EndpointHost, Provenance: "published_port", Priority: 100}},
		Settings:           ReceiverSettings{UsernameEnv: "AR_IMMS_PG_USER", PasswordEnv: "AR_IMMS_PG_PASSWORD"},
	}
	registry, err := NewRegistry(redis, postgres)
	if err != nil {
		t.Fatal(err)
	}
	registeredProfiles := registry.Profiles()
	if len(registeredProfiles) != 2 || registeredProfiles[0].ID != "postgres" || registeredProfiles[1].ID != "redis" {
		t.Fatalf("registry profile snapshot=%+v", registeredProfiles)
	}
	layer, receivers, err := registry.DockerCollectorLayer("windows", dockerEndpoint("windows"), "core-node-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(receivers) != 1 || receivers[0] != "receiver_creator" {
		t.Fatalf("receiver pipeline entries=%v", receivers)
	}
	configured := layer.Document["receivers"].(map[string]any)["receiver_creator"].(map[string]any)["receivers"].(map[string]any)
	if configured["redis/alternate_port"] == nil || configured["postgresql/alternate_port"] == nil {
		t.Fatalf("registry profiles were not both projected: %#v", configured)
	}
}

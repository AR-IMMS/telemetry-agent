package application

import (
	"testing"

	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
)

func TestDiagnoseEndpointDistinguishesMissingAmbiguousAndResolved(t *testing.T) {
	profile := domain.RedisProfile()
	tests := []struct {
		name      string
		osName    string
		endpoints []inventoryctx.Endpoint
		want      EndpointState
		count     int
	}{
		{name: "no host mapping on Windows", osName: "windows", endpoints: []inventoryctx.Endpoint{{Address: "172.19.0.3", Port: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}}, want: EndpointMissing},
		{name: "one published target", osName: "windows", endpoints: []inventoryctx.Endpoint{{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port", ContainerPort: 6379}}, want: EndpointResolved, count: 1},
		{name: "two published targets", osName: "windows", endpoints: []inventoryctx.Endpoint{{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port", ContainerPort: 6379}, {Address: "127.0.0.1", Port: 16380, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port", ContainerPort: 6379}}, want: EndpointAmbiguous, count: 2},
		{name: "host mapping wins over multiple network addresses", osName: "linux", endpoints: []inventoryctx.Endpoint{{Address: "172.19.0.3", Port: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}, {Address: "172.20.0.3", Port: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}, {Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port", ContainerPort: 6379}}, want: EndpointResolved, count: 1},
		{name: "multiple unbound networks are ambiguous", osName: "linux", endpoints: []inventoryctx.Endpoint{{Address: "172.19.0.3", Port: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}, {Address: "172.20.0.3", Port: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network", ContainerPort: 6379}}, want: EndpointAmbiguous, count: 2},
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
	profile := domain.RedisProfile()
	attributes := map[string]string{"label." + domain.LabelMonitoringPort: "6380"}
	endpoints := []inventoryctx.Endpoint{
		{Address: "172.20.0.4", NetworkName: "zeta", NetworkPriority: 2, Port: 6380, ContainerPort: 6380, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network"},
		{Address: "172.19.0.4", NetworkName: "alpha", NetworkPriority: 1, Port: 6380, ContainerPort: 6380, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network"},
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
			got := DiagnoseEndpointWithAttributes(domain.RedisProfile(), "linux", nil, nil, map[string]string{"label." + domain.LabelMonitoringPort: value})
			if got.State != EndpointInvalid || got.EvidenceCode != "invalid_target_port" {
				t.Fatalf("invalid target port diagnosis=%+v", got)
			}
		})
	}
}

func TestDiagnoseEndpointUsesNetworkNameForPriorityTie(t *testing.T) {
	profile := domain.RedisProfile()
	endpoints := []inventoryctx.Endpoint{
		{Address: "172.20.0.4", NetworkName: "zeta", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network"},
		{Address: "172.19.0.4", NetworkName: "alpha", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network"},
	}
	got := DiagnoseEndpoint(profile, "linux", endpoints, nil)
	if got.State != EndpointResolved || got.Endpoint.NetworkName != "alpha" {
		t.Fatalf("network priority tie diagnosis=%+v, want alphabetical network alpha", got)
	}
}

func TestDiagnoseEndpointFailsClosedWithoutNetworkIdentity(t *testing.T) {
	profile := domain.RedisProfile()
	endpoints := []inventoryctx.Endpoint{
		{Address: "172.20.0.4", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network"},
		{Address: "172.19.0.4", Port: 6379, ContainerPort: 6379, Protocol: "tcp", Scope: inventoryctx.EndpointContainer, Provenance: "container_network"},
	}
	got := DiagnoseEndpoint(profile, "linux", endpoints, nil)
	if got.State != EndpointAmbiguous || got.CandidateCount != 2 {
		t.Fatalf("missing network identity diagnosis=%+v, want fail-closed ambiguity", got)
	}
}

func TestDiagnoseEndpointRequiresMatchingOTelScrapeFailureForUnreachable(t *testing.T) {
	profile := domain.RedisProfile()
	endpoint := inventoryctx.Endpoint{Address: "127.0.0.1", Port: 16379, Protocol: "tcp", Scope: inventoryctx.EndpointHost, Provenance: "published_port", ContainerPort: 6379}
	withoutEvidence := DiagnoseEndpoint(profile, "windows", []inventoryctx.Endpoint{endpoint}, nil)
	if withoutEvidence.State != EndpointResolved {
		t.Fatalf("discovered endpoint state=%q, want resolved but unverified", withoutEvidence.State)
	}
	evidence := []CollectorScrapeEvidence{{ReceiverID: "redis/alternate_port", Endpoint: "127.0.0.1:16379", FailureCode: "connection_failed"}}
	got := DiagnoseEndpoint(profile, "windows", []inventoryctx.Endpoint{endpoint}, evidence)
	if got.State != EndpointUnreachable || got.EvidenceSource != EvidenceCollectorScrapeError {
		t.Fatalf("diagnosis=%+v, want OTel-evidenced unreachable endpoint", got)
	}
}

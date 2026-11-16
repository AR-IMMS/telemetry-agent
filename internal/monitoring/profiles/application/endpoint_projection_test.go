package application

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
)

func TestDiagnoseEndpointUsesMonitoringOwnedEndpointProjection(t *testing.T) {
	endpoint := EndpointObservation{
		Address:       "127.0.0.1",
		Port:          16379,
		ContainerPort: 6379,
		Protocol:      "tcp",
		Scope:         domain.EndpointHost,
		Provenance:    "published_port",
	}

	got := DiagnoseEndpoint(domain.RedisProfile(), "windows", []EndpointObservation{endpoint}, nil)
	if got.State != EndpointResolved || got.Endpoint.Address != endpoint.Address || got.Endpoint.Port != endpoint.Port {
		t.Fatalf("diagnostic = %+v, want resolved projected endpoint %+v", got, endpoint)
	}
}

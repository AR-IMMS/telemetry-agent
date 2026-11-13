package opampserver

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/protobufs"
)

func TestRenderGatewayConfigIsNarrowAndDeterministic(t *testing.T) {
	policy := coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 7, GatewayEndpoint: "gateway.example:4317"}
	body, hash, err := RenderGatewayConfig(policy)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "exporters:\n  otlp/gateway:\n    endpoint: \"gateway.example:4317\"\n" {
		t.Fatalf("rendered config = %q", body)
	}
	if hash != sha256.Sum256(body) {
		t.Fatal("rendered config hash is not SHA-256 of the exact body")
	}
	if strings.Contains(string(body), "receivers:") || strings.Contains(string(body), "service:") {
		t.Fatal("remote config included Agent-owned receiver or pipeline settings")
	}
}

func TestRenderGatewayConfigRejectsUnsupportedPolicy(t *testing.T) {
	for _, policy := range []coreapi.DesiredGatewayPolicy{
		{SchemaVersion: "ar-imms/gateway-policy/v2", Revision: 1, GatewayEndpoint: "gateway.example:4317"},
		{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 0, GatewayEndpoint: "gateway.example:4317"},
		{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "https://gateway.example:4317/path"},
	} {
		if _, _, err := RenderGatewayConfig(policy); err == nil {
			t.Fatalf("unsupported policy was accepted: %+v", policy)
		}
	}
}

func TestNewRejectsCleartextOutsideLoopback(t *testing.T) {
	_, err := New(Options{ListenEndpoint: "0.0.0.0:4319", CoreClient: func(string) (coreapi.AgentManagementAPI, error) { return nil, nil }})
	if err == nil {
		t.Fatal("cleartext OpAMP server bound to a non-loopback address")
	}
}

func TestBearerTokenParsing(t *testing.T) {
	for _, test := range []struct {
		header string
		wantOK bool
	}{
		{header: "Bearer opaque-secret", wantOK: true},
		{header: "bearer opaque-secret", wantOK: true},
		{header: "", wantOK: false},
		{header: "Bearer", wantOK: false},
		{header: "Bearer  opaque-secret", wantOK: false},
		{header: "Bearer opaque secret", wantOK: false},
		{header: "Basic opaque-secret", wantOK: false},
	} {
		got, ok := bearerToken(test.header)
		if ok != test.wantOK || ok && got != "opaque-secret" {
			t.Fatalf("bearerToken(%q) = %q, %t", test.header, got, ok)
		}
	}
}

func TestNewAllowsCleartextLoopback(t *testing.T) {
	if _, err := New(Options{ListenEndpoint: "localhost:4319", ListenPath: "/v1/opamp", CoreClient: func(string) (coreapi.AgentManagementAPI, error) { return nil, nil }}); err != nil {
		t.Fatalf("loopback fixture should be allowed: %v", err)
	}
}

func TestHandleMessageDoesNotOfferConfigWithoutAcceptRemoteConfigCapability(t *testing.T) {
	api := &capabilityTestClient{}
	server := &Server{options: Options{Logger: slog.Default()}, sent: make(map[string]map[string]int64)}
	uid, err := uuid.Parse("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	if err != nil {
		t.Fatal(err)
	}
	uidBytes, err := uid.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	response := server.handleMessage(context.Background(), api, &protobufs.AgentToServer{InstanceUid: uidBytes})
	if response == nil || response.RemoteConfig != nil || api.sessionCalls != 0 {
		t.Fatalf("response=%+v, Core session calls=%d", response, api.sessionCalls)
	}
}

type capabilityTestClient struct{ sessionCalls int }

func (c *capabilityTestClient) GetSession(context.Context) (coreapi.AgentManagementSession, error) {
	c.sessionCalls++
	return coreapi.AgentManagementSession{}, nil
}
func (*capabilityTestClient) BindOpAMPInstance(context.Context, string) (string, error) {
	return "", nil
}
func (*capabilityTestClient) RecordPublication(context.Context, coreapi.PublicationRequest) (coreapi.ConfigurationPublication, error) {
	return coreapi.ConfigurationPublication{}, nil
}
func (*capabilityTestClient) ResolvePublication(context.Context, string, string) (coreapi.ConfigurationPublication, error) {
	return coreapi.ConfigurationPublication{}, nil
}
func (*capabilityTestClient) ReportStatus(context.Context, coreapi.SupervisorStatusReport) error {
	return nil
}

package coreapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/mockcore"
)

func TestAgentManagementClientUsesEnrollmentBearerAndBindsInstance(t *testing.T) {
	const token = "opaque-enrollment-bearer"
	mock, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: "11111111-2222-4333-8444-555555555555",
		NodeID: "33333333-4444-4555-8666-777777777777",
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(mock)
	defer server.Close()
	client, err := coreapi.NewAgentManagementClient(server.URL, token, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.GetSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.InstallationID != "11111111-2222-4333-8444-555555555555" || session.NodeID != "33333333-4444-4555-8666-777777777777" || session.OpAMPInstanceUID != "" || session.DesiredPolicy.Revision != 1 {
		t.Fatalf("session = %+v", session)
	}
	const instanceUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if bound, err := client.BindOpAMPInstance(context.Background(), instanceUID); err != nil || bound != instanceUID {
		t.Fatalf("bind result = %q, %v", bound, err)
	}
	if bound, err := client.BindOpAMPInstance(context.Background(), instanceUID); err != nil || bound != instanceUID {
		t.Fatalf("idempotent bind result = %q, %v", bound, err)
	}
	_, err = client.BindOpAMPInstance(context.Background(), "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff")
	var apiError *coreapi.ControlAPIError
	if !errors.As(err, &apiError) || apiError.Status != 409 || apiError.Code != "OPAMP_INSTANCE_CONFLICT" {
		t.Fatalf("conflicting bind error = %#v", err)
	}
	if err := client.ReportStatus(context.Background(), coreapi.SupervisorStatusReport{
		InstanceUID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff", DesiredRevision: 1, Status: "APPLIED", ConfigHash: strings.Repeat("a", 64),
	}); !errors.As(err, &apiError) || apiError.Status != 403 || apiError.Code != "OPAMP_INSTANCE_MISMATCH" {
		t.Fatalf("mismatched status identity error = %#v", err)
	}
	report := coreapi.SupervisorStatusReport{
		InstanceUID: instanceUID, DesiredRevision: 1, Status: "APPLIED", ConfigHash: strings.Repeat("a", 64),
	}
	if err := client.ReportStatus(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if err := client.ReportStatus(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	observations := mock.Statuses()
	if len(observations) != 1 || observations[0].InstallationID != session.InstallationID || observations[0].Report.AppliedRevision != 0 {
		t.Fatalf("status observations = %+v", observations)
	}
	mock.Revoke(token)
	_, err = client.GetSession(context.Background())
	apiError = nil
	if !errors.As(err, &apiError) || apiError.Status != 401 || apiError.Code != "UNAUTHORIZED" {
		t.Fatalf("revoked credential error = %#v", err)
	}
}

func TestAgentManagementClientRejectsInsecureCoreURL(t *testing.T) {
	if _, err := coreapi.NewAgentManagementClient("http://core.example", "token", nil); err == nil {
		t.Fatal("insecure Core URL was accepted")
	}
}

func TestMockCoreRejectsMalformedStatusHash(t *testing.T) {
	const token = "status-contract-test-token"
	const instanceUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	mock, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: "11111111-2222-4333-8444-555555555555",
		NodeID: "33333333-4444-4555-8666-777777777777", InstanceUID: instanceUID,
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(mock)
	defer server.Close()
	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/agent-management/status", strings.NewReader(`{"instance_uid":"`+instanceUID+`","desired_revision":1,"status":"APPLIED","config_hash":"`+strings.Repeat("z", 64)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || response.Header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("malformed status response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

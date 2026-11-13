package coreapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	publication, err := client.RecordPublication(context.Background(), coreapi.PublicationRequest{
		InstanceUID: instanceUID, PolicySchemaVersion: "ar-imms/gateway-policy/v1", PolicyRevision: 1, ConfigHash: strings.Repeat("a", 64),
	})
	if err != nil || publication.PublishedAt.IsZero() {
		t.Fatalf("record publication = %+v, %v", publication, err)
	}
	resolved, err := client.ResolvePublication(context.Background(), instanceUID, publication.ConfigHash)
	if err != nil || resolved.PolicyRevision != 1 || resolved.ConfigHash != publication.ConfigHash {
		t.Fatalf("resolve publication = %+v, %v", resolved, err)
	}
	if err := client.ReportStatus(context.Background(), coreapi.SupervisorStatusReport{
		InstanceUID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff", DesiredRevision: 1, PublicationRevision: 1, AppliedRevision: 1, Status: "APPLIED", ConfigHash: strings.Repeat("a", 64),
	}); !errors.As(err, &apiError) || apiError.Status != 403 || apiError.Code != "OPAMP_INSTANCE_MISMATCH" {
		t.Fatalf("mismatched status identity error = %#v", err)
	}
	report := coreapi.SupervisorStatusReport{
		InstanceUID: instanceUID, DesiredRevision: 1, PublicationRevision: 1, AppliedRevision: 1, Status: "APPLIED", ConfigHash: strings.Repeat("a", 64),
	}
	if err := client.ReportStatus(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if err := client.ReportStatus(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	observations := mock.Statuses()
	if len(observations) != 1 || observations[0].InstallationID != session.InstallationID || observations[0].Report.AppliedRevision != 1 {
		t.Fatalf("status observations = %+v", observations)
	}
	if err := mock.SetPolicy(session.InstallationID, coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 2, GatewayEndpoint: "127.0.0.2:4317"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RecordPublication(context.Background(), coreapi.PublicationRequest{
		InstanceUID: instanceUID, PolicySchemaVersion: "ar-imms/gateway-policy/v1", PolicyRevision: 2, ConfigHash: strings.Repeat("b", 64),
	}); err != nil {
		t.Fatal(err)
	}
	rollback := coreapi.SupervisorStatusReport{InstanceUID: instanceUID, DesiredRevision: 2, PublicationRevision: 1, AppliedRevision: 1, Status: "APPLIED", ConfigHash: strings.Repeat("a", 64)}
	if err := client.ReportStatus(context.Background(), rollback); err != nil {
		t.Fatalf("report previous published hash during rollback: %v", err)
	}
	if len(mock.Statuses()) != 2 {
		t.Fatalf("status count after rollback report = %d, want 2", len(mock.Statuses()))
	}
	if _, err := client.ResolvePublication(context.Background(), instanceUID, strings.Repeat("c", 64)); err == nil {
		t.Fatal("unknown publication hash resolved")
	}
	mock.Revoke(token)
	_, err = client.GetSession(context.Background())
	apiError = nil
	if !errors.As(err, &apiError) || apiError.Status != 401 || apiError.Code != "UNAUTHORIZED" {
		t.Fatalf("revoked credential error = %#v", err)
	}
}

func TestMockCorePublicationAndStatusSurviveServerRestart(t *testing.T) {
	const token = "persistent-mock-core-token"
	const installationID = "11111111-2222-4333-8444-555555555555"
	const instanceUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	statePath := filepath.Join(t.TempDir(), "mock-core-state.json")
	seed := mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: installationID,
		NodeID: "33333333-4444-4555-8666-777777777777",
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	}
	first, err := mockcore.NewPersistent(seed, statePath)
	if err != nil {
		t.Fatal(err)
	}
	serve := func(handler http.Handler) (*httptest.Server, *coreapi.AgentManagementClient) {
		httpServer := httptest.NewTLSServer(handler)
		client, clientErr := coreapi.NewAgentManagementClient(httpServer.URL, token, httpServer.Client())
		if clientErr != nil {
			httpServer.Close()
			t.Fatal(clientErr)
		}
		return httpServer, client
	}
	firstHTTP, client := serve(first)
	if _, err := client.BindOpAMPInstance(context.Background(), instanceUID); err != nil {
		t.Fatal(err)
	}
	publication, err := client.RecordPublication(context.Background(), coreapi.PublicationRequest{
		InstanceUID: instanceUID, PolicySchemaVersion: "ar-imms/gateway-policy/v1", PolicyRevision: 1, ConfigHash: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ReportStatus(context.Background(), coreapi.SupervisorStatusReport{
		InstanceUID: instanceUID, DesiredRevision: 1, PublicationRevision: 1, AppliedRevision: 1,
		Status: "APPLIED", ConfigHash: publication.ConfigHash,
	}); err != nil {
		t.Fatal(err)
	}
	firstHTTP.Close()
	fixtureBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fixtureBytes), token) {
		t.Fatal("persistent Mock Core fixture contains the bearer value")
	}

	second, err := mockcore.NewPersistent(seed, statePath)
	if err != nil {
		t.Fatal(err)
	}
	secondHTTP, restartedClient := serve(second)
	defer secondHTTP.Close()
	session, err := restartedClient.GetSession(context.Background())
	if err != nil || session.OpAMPInstanceUID != instanceUID {
		t.Fatalf("restarted session = %+v, %v", session, err)
	}
	resolved, err := restartedClient.ResolvePublication(context.Background(), instanceUID, publication.ConfigHash)
	if err != nil || resolved.PublishedAt != publication.PublishedAt {
		t.Fatalf("restarted publication = %+v, %v", resolved, err)
	}
	if statuses := second.Statuses(); len(statuses) != 1 || statuses[0].Report.Status != "APPLIED" {
		t.Fatalf("restarted statuses = %+v", statuses)
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

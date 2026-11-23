package mockcore

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
)

func TestReplaceCredentialRevokesPriorBearerAndPersistsReplacement(t *testing.T) {
	const (
		installationID = "11111111-2222-4333-8444-555555555555"
		nodeID         = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
		oldToken       = "mock-core-old-agent-token"
		newToken       = "mock-core-new-agent-token"
	)
	statePath := filepath.Join(t.TempDir(), "mock-core.json")
	server, err := NewPersistent(ProvisionedInstallation{
		AccessToken: oldToken, InstallationID: installationID, NodeID: nodeID,
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	}, statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ReplaceCredential(oldToken, newToken); err != nil {
		t.Fatalf("replace credential: %v", err)
	}

	restarted, err := NewPersistent(ProvisionedInstallation{
		AccessToken: oldToken, InstallationID: installationID, NodeID: nodeID,
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	}, statePath)
	if err != nil {
		t.Fatalf("restart persistent Mock Core: %v", err)
	}
	if got := sessionStatus(t, restarted, oldToken); got != http.StatusUnauthorized {
		t.Fatalf("old bearer status = %d, want %d", got, http.StatusUnauthorized)
	}
	if got := sessionStatus(t, restarted, newToken); got != http.StatusOK {
		t.Fatalf("replacement bearer status = %d, want %d", got, http.StatusOK)
	}
	content, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), oldToken) || strings.Contains(string(content), newToken) {
		t.Fatal("persistent Mock Core state contains bearer credential material")
	}
}

func sessionStatus(t *testing.T, server http.Handler, token string) int {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-management/session", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response.Code
}

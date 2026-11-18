package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/mockfleet"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func TestEnrollmentApplicationStoresCredentialFromProvisionalMockFleetExchange(t *testing.T) {
	if !canUseProtectedCredentialStoreForTest() {
		if os.Getenv("AR_IMMS_REQUIRE_PROTECTED_ENROLLMENT_TEST") == "1" {
			t.Fatal("required protected enrollment test has no elevated Windows token")
		}
		t.Skip("protected Windows credential-store integration requires an elevated process")
	}
	const (
		agentID     = "10000000-0000-4000-8000-000000000001"
		nodeID      = "10000000-0000-4000-8000-000000000002"
		inviteToken = "test-only-invitation"
	)
	fleet := mockfleet.New()
	if err := fleet.SeedInvitation(inviteToken, nodeID); err != nil {
		t.Fatal(err)
	}
	var receivedExchange []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedExchange, _ = io.ReadAll(request.Body)
		request.Body = io.NopCloser(bytes.NewReader(receivedExchange))
		fleet.ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	origin, err := enrollment.ParseCoreOrigin("https://fleet.test.invalid")
	if err != nil {
		t.Fatal(err)
	}
	statePath := t.TempDir() + "/agent-state.db"
	parsedAgentID, err := installationid.ParseInstallationID(agentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(parsedAgentID); err != nil {
		t.Fatalf("initialize Agent installation identity: %v", err)
	}
	repository := sqlitefixture.NewEnrollmentStore(statePath)
	credentials := credentialstore.New(statePath)
	service := enrollment.Service{
		Repository:  repository,
		Credentials: credentials,
		Client:      provisionalMockFleetEnrollmentClient{endpoint: server.URL},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := service.Start(ctx, origin, enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: agentID},
		Agent:        enrollment.Agent{Version: "test"},
		Environment:  enrollment.Environment{OS: "linux", Architecture: "amd64"},
		Metadata: map[string]any{
			"host_id": "host-evidence",
		},
	}, inviteToken)
	if err != nil {
		t.Fatalf("run enrollment application against provisional fixture: %v", err)
	}
	if result.InstallationID != agentID || result.Binding.NodeID != nodeID || result.Binding.Status != "active" {
		t.Fatalf("enrollment result = %+v, want installation %s bound to fixture node %s", result, agentID, nodeID)
	}

	attempt, err := repository.LoadAttempt()
	if err != nil {
		t.Fatalf("load persisted enrollment attempt: %v", err)
	}
	if attempt.Status != enrollment.AttemptRegistered || attempt.Result == nil || attempt.Result.Binding.NodeID != nodeID {
		t.Fatalf("persisted attempt = %+v, want registered fixture binding", attempt)
	}
	var exchange struct {
		AttemptID string `json:"enrollment_attempt_id"`
		AgentID   string `json:"agent_id"`
		Token     string `json:"invitation_token"`
		Evidence  struct {
			HostID string `json:"host_id"`
		} `json:"identity_evidence"`
	}
	if err := json.Unmarshal(receivedExchange, &exchange); err != nil {
		t.Fatalf("decode captured fixture exchange: %v", err)
	}
	if exchange.AttemptID != attempt.Payload.AttemptID || exchange.AgentID != agentID || exchange.Token != inviteToken || exchange.Evidence.HostID != "host-evidence" {
		t.Fatalf("provisional fixture exchange = %+v, want persisted attempt, installation identity, invitation, and host evidence", exchange)
	}
	wantCredential := "mock-credential-" + attempt.Payload.AttemptID
	if got, err := credentials.Get(attempt.AccessCredentialRef); err != nil || got != wantCredential {
		t.Fatalf("stored Agent credential = %q, err=%v, want protected fixture credential", got, err)
	}
	if _, err := credentials.Get(attempt.BootstrapCredentialRef); !errors.Is(err, enrollment.ErrCredentialNotFound) {
		t.Fatalf("bootstrap credential lookup error = %v, want deleted credential", err)
	}
	if strings.Contains(attempt.BootstrapCredentialRef, inviteToken) || strings.Contains(attempt.AccessCredentialRef, "mock-credential-") {
		t.Fatal("credential material appeared in the persisted enrollment references")
	}
	for _, file := range enrollmentDatabaseFiles(t, statePath) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read enrollment database file %s: %v", file, err)
		}
		if bytes.Contains(content, []byte(inviteToken)) || bytes.Contains(content, []byte(wantCredential)) {
			clear(content)
			t.Fatalf("SQLite file %s contains enrollment credential material", file)
		}
		clear(content)
	}
}

// provisionalMockFleetEnrollmentClient is test-only glue between the current
// Core-shaped enrollment application port and the illustrative Mock Fleet
// exchange. It does not define or verify a production Fleet contract.
type provisionalMockFleetEnrollmentClient struct {
	endpoint string
}

func (c provisionalMockFleetEnrollmentClient) Enroll(ctx context.Context, origin enrollment.CoreOrigin, request enrollment.EnrollmentRequest) (enrollment.EnrollmentResponse, error) {
	body, err := json.Marshal(struct {
		SchemaVersion int    `json:"schema_version"`
		AttemptID     string `json:"enrollment_attempt_id"`
		AgentID       string `json:"agent_id"`
		Token         string `json:"invitation_token"`
		Evidence      struct {
			HostID string `json:"host_id,omitempty"`
		} `json:"identity_evidence,omitempty"`
	}{
		SchemaVersion: 1,
		AttemptID:     request.AttemptID,
		AgentID:       request.Installation.ID,
		Token:         request.BootstrapToken,
		Evidence: struct {
			HostID string `json:"host_id,omitempty"`
		}{HostID: metadataString(request.Metadata, "host_id")},
	})
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("encode provisional fixture exchange: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/enrollments/exchange", bytes.NewReader(body))
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("create provisional fixture request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("send provisional fixture request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("provisional fixture returned HTTP %d", response.StatusCode)
	}
	var issued struct {
		AgentID    string `json:"agent_id"`
		NodeID     string `json:"node_id"`
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(response.Body).Decode(&issued); err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("decode provisional fixture response: %w", err)
	}
	if issued.AgentID != request.Installation.ID {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("provisional fixture Agent identity mismatch")
	}
	// The current application port validates a Core-shaped result. These
	// synthetic values exercise its persistence pipeline only; they make no
	// claim that Fleet returns Core bindings or a Core control-plane endpoint.
	return enrollment.EnrollmentResponse{
		EnrollmentID:   request.AttemptID,
		InstallationID: request.Installation.ID,
		Binding:        enrollment.Binding{ID: "10000000-0000-4000-8000-000000000003", NodeID: issued.NodeID, Status: "active"},
		Credential:     enrollment.Credential{Type: "bearer", AccessToken: issued.Credential},
		ControlPlane:   enrollment.ControlPlane{BaseURL: origin.String(), PollIntervalSeconds: 30},
	}, nil
}

func enrollmentDatabaseFiles(t *testing.T, statePath string) []string {
	t.Helper()
	files, err := filepath.Glob(statePath + "*")
	if err != nil {
		t.Fatalf("list enrollment database files: %v", err)
	}
	return files
}

func metadataString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

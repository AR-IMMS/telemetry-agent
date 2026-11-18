package mockfleet

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeReplaysSameResultForSameAttempt(t *testing.T) {
	server := New()
	if err := server.SeedInvitation("invitation-token", "node-1"); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	request := []byte(`{"schema_version":1,"enrollment_attempt_id":"attempt-1","agent_id":"agent-1","invitation_token":"invitation-token","identity_evidence":{"host_id":"host-1","compute_type":"VM","orchestrator":"NONE","deployment_scope":"HOST_SERVICE"}}`)
	firstStatus, firstBody := exchange(t, httpServer.URL, request)
	secondStatus, secondBody := exchange(t, httpServer.URL, request)

	if firstStatus != http.StatusCreated || secondStatus != http.StatusCreated {
		t.Fatalf("same-attempt exchange statuses = %d, %d; want %d, %d", firstStatus, secondStatus, http.StatusCreated, http.StatusCreated)
	}
	if !bytes.Equal(firstBody, secondBody) {
		t.Fatalf("same-attempt exchange changed its result:\nfirst:  %s\nsecond: %s", firstBody, secondBody)
	}
	var result struct {
		SchemaVersion int    `json:"schema_version"`
		AgentID       string `json:"agent_id"`
		NodeID        string `json:"node_id"`
		CredentialID  string `json:"credential_id"`
		Credential    string `json:"credential"`
		OpAMP         string `json:"opamp_endpoint"`
	}
	if err := json.Unmarshal(firstBody, &result); err != nil {
		t.Fatalf("decode enrollment result: %v", err)
	}
	if result.SchemaVersion != 1 || result.AgentID != "agent-1" || result.NodeID != "node-1" || result.CredentialID == "" || result.Credential == "" || result.OpAMP != "wss://fleet.test.invalid/opamp" {
		t.Fatalf("unexpected provisional enrollment result: %+v", result)
	}
}

func TestExchangeRejectsChangedRequestForExistingAttempt(t *testing.T) {
	server := New()
	if err := server.SeedInvitation("invitation-token", "node-1"); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	first := []byte(`{"schema_version":1,"enrollment_attempt_id":"attempt-1","agent_id":"agent-1","invitation_token":"invitation-token","identity_evidence":{"host_id":"host-1","compute_type":"VM","orchestrator":"NONE","deployment_scope":"HOST_SERVICE"}}`)
	changed := []byte(`{"schema_version":1,"enrollment_attempt_id":"attempt-1","agent_id":"agent-2","invitation_token":"invitation-token","identity_evidence":{"host_id":"host-1","compute_type":"VM","orchestrator":"NONE","deployment_scope":"HOST_SERVICE"}}`)
	if status, _ := exchange(t, httpServer.URL, first); status != http.StatusCreated {
		t.Fatalf("first exchange status = %d; want %d", status, http.StatusCreated)
	}
	if status, _ := exchange(t, httpServer.URL, changed); status != http.StatusConflict {
		t.Fatalf("changed same-attempt exchange status = %d; want provisional conflict %d", status, http.StatusConflict)
	}
}

func TestExchangeRejectsInvitationReplayWithDifferentAttempt(t *testing.T) {
	server := New()
	if err := server.SeedInvitation("invitation-token", "node-1"); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	first := []byte(`{"schema_version":1,"enrollment_attempt_id":"attempt-1","agent_id":"agent-1","invitation_token":"invitation-token","identity_evidence":{"host_id":"host-1","compute_type":"VM","orchestrator":"NONE","deployment_scope":"HOST_SERVICE"}}`)
	replay := []byte(`{"schema_version":1,"enrollment_attempt_id":"attempt-2","agent_id":"agent-2","invitation_token":"invitation-token","identity_evidence":{"host_id":"host-2","compute_type":"BARE_METAL","orchestrator":"NONE","deployment_scope":"HOST_SERVICE"}}`)
	if status, _ := exchange(t, httpServer.URL, first); status != http.StatusCreated {
		t.Fatalf("first exchange status = %d; want %d", status, http.StatusCreated)
	}
	if status, _ := exchange(t, httpServer.URL, replay); status != http.StatusConflict {
		t.Fatalf("replayed invitation status = %d; want provisional conflict %d", status, http.StatusConflict)
	}
}

func exchange(t *testing.T, baseURL string, body []byte) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/enrollments/exchange", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}

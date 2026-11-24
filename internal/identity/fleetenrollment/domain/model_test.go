package domain

import (
	"encoding/json"
	"strings"
	"testing"

	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

func TestFleetEnrollmentResponseRequiresMatchingIdentityAndOpAMPOnlyScope(t *testing.T) {
	agentID := "11111111-1111-4111-8111-111111111111"
	nodeID := "22222222-2222-4222-8222-222222222222"
	response := EnrollmentResponse{
		AgentID: agentID, NodeID: nodeID,
		CredentialID: "cred_01JEXAMPLE", Credential: "secret",
		CredentialScope: "opamp.connect", OpAMPEndpoint: "wss://fleet.example.test/opamp",
	}
	if err := response.Validate(agentID); err != nil {
		t.Fatalf("valid Fleet response rejected: %v", err)
	}

	response.AgentID = "33333333-3333-4333-8333-333333333333"
	if err := response.Validate(agentID); err == nil {
		t.Fatal("mismatched Fleet agent ID accepted")
	}
	response.AgentID = agentID
	response.CredentialScope = "core.api"
	if err := response.Validate(agentID); err == nil {
		t.Fatal("non-OpAMP Fleet credential scope accepted")
	}
}

func TestFleetEnrollmentResponseValidatesSeparateOTLPCredential(t *testing.T) {
	agentID := "11111111-1111-4111-8111-111111111111"
	response := EnrollmentResponse{
		AgentID: agentID, NodeID: "22222222-2222-4222-8222-222222222222",
		CredentialID: "opamp-id", Credential: "opamp-secret", CredentialScope: "opamp.connect",
		OpAMPEndpoint:  "wss://fleet.example.test/opamp",
		OTLPCredential: &OTLPCredential{ID: "otlp-id", Token: "otlp-secret", Scope: "otlp.write"},
	}
	if err := response.Validate(agentID); err != nil {
		t.Fatalf("valid separate OTLP credential rejected: %v", err)
	}
	response.OTLPCredential.Scope = "opamp.connect"
	if err := response.Validate(agentID); err == nil {
		t.Fatal("OTLP credential with OpAMP scope accepted")
	}
	response.OTLPCredential.Scope = "otlp.write"
	response.OTLPCredential.Token = ""
	if err := response.Validate(agentID); err == nil {
		t.Fatal("partial OTLP credential accepted")
	}
}

func TestFleetEnrollmentResponseRejectsInsecureOrMalformedOpAMPEndpoint(t *testing.T) {
	agentID := "11111111-1111-4111-8111-111111111111"
	response := EnrollmentResponse{
		AgentID: agentID, NodeID: "22222222-2222-4222-8222-222222222222",
		CredentialID: "cred", Credential: "secret", CredentialScope: "opamp.connect",
		OpAMPEndpoint: "ws://fleet.example.test/opamp",
	}
	if err := response.Validate(agentID); err == nil {
		t.Fatal("insecure non-loopback ws endpoint accepted")
	}
	response.OpAMPEndpoint = "wss://user:password@fleet.example.test/opamp"
	if err := response.Validate(agentID); err == nil {
		t.Fatal("OpAMP endpoint user info accepted")
	}
}

func TestFleetExchangeRequestAndStableAgentID(t *testing.T) {
	agentID := "11111111-1111-4111-8111-111111111111"
	parsed, err := installationid.ParseInstallationID(agentID)
	if err != nil || parsed.String() != agentID {
		t.Fatalf("Agent identity parse = %v, %v", parsed, err)
	}
	request := ExchangeRequest{AttemptID: agentID, AgentID: agentID, InvitationToken: "invite-secret", RecoveryProof: strings.Repeat("p", 32)}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid exchange request rejected: %v", err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "instance_uid") {
		t.Fatalf("exchange request conflated Agent and Supervisor identity: %s", encoded)
	}
}

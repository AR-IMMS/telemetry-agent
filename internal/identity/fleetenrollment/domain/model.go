package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

// Origin is the trusted Fleet HTTPS origin for enrollment operations.
type Origin struct{ value string }

// ParseOrigin accepts only an HTTPS origin without path, query, fragment or user info.
func ParseOrigin(raw string) (Origin, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Origin{}, fmt.Errorf("fleet URL must be an HTTPS origin without credentials or path")
	}
	secureScheme := strings.EqualFold(parsed.Scheme, "https") || strings.EqualFold(parsed.Scheme, "http") && isLoopback(parsed.Hostname())
	if !secureScheme || parsed.Hostname() == "" ||
		parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Origin{}, fmt.Errorf("fleet URL must be an HTTPS origin without credentials or path")
	}
	parsed.Path, parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", "", ""
	parsed.User = nil
	return Origin{value: strings.TrimSuffix(parsed.String(), "/")}, nil
}

// String returns the normalized trusted Fleet origin.
func (o Origin) String() string { return o.value }

// ExchangeRequest is the immutable, secret-bearing Agent-to-Fleet request.
type ExchangeRequest struct {
	AttemptID       string `json:"enrollment_attempt_id"`
	AgentID         string `json:"agent_id"`
	InvitationToken string `json:"invitation_token"`
	RecoveryProof   string `json:"recovery_proof"`
}

// Validate checks local identity fields and required recovery material.
func (r ExchangeRequest) Validate() error {
	if _, err := installationid.ParseInstallationID(r.AttemptID); err != nil {
		return fmt.Errorf("fleet enrollment attempt ID is invalid")
	}
	if _, err := installationid.ParseInstallationID(r.AgentID); err != nil {
		return fmt.Errorf("fleet Agent ID is invalid")
	}
	if strings.TrimSpace(r.InvitationToken) == "" || len(r.InvitationToken) > 4096 {
		return fmt.Errorf("fleet invitation token is required and must be bounded")
	}
	if len(r.RecoveryProof) < 32 || len(r.RecoveryProof) > 256 {
		return fmt.Errorf("fleet recovery proof must contain 32 to 256 characters")
	}
	return nil
}

// RecoveryRequest proves authority to recover one original exchange response.
type RecoveryRequest struct {
	AttemptID     string `json:"enrollment_attempt_id"`
	AgentID       string `json:"agent_id"`
	RecoveryProof string `json:"recovery_proof"`
}

// EnrollmentResponse is the Fleet exchange result, including the one-time credential.
type EnrollmentResponse struct {
	AgentID             string     `json:"agent_id"`
	NodeID              string     `json:"node_id"`
	CredentialID        string     `json:"credential_id"`
	Credential          string     `json:"credential"`
	CredentialScope     string     `json:"credential_scope"`
	OpAMPEndpoint       string     `json:"opamp_endpoint"`
	CredentialExpiresAt *time.Time `json:"credential_expires_at"`
}

// Result is the secret-free Fleet enrollment summary persisted in local state.
type Result struct {
	AgentID             string     `json:"agent_id"`
	NodeID              string     `json:"node_id"`
	CredentialID        string     `json:"credential_id"`
	CredentialScope     string     `json:"credential_scope"`
	OpAMPEndpoint       string     `json:"opamp_endpoint"`
	CredentialExpiresAt *time.Time `json:"credential_expires_at"`
}

// Validate rejects credential material with an unexpected identity, scope or endpoint.
func (r EnrollmentResponse) Validate(expectedAgentID string) error {
	if _, err := installationid.ParseInstallationID(expectedAgentID); err != nil || r.AgentID != expectedAgentID {
		return fmt.Errorf("fleet enrollment response Agent ID does not match this installation")
	}
	if _, err := installationid.ParseInstallationID(r.NodeID); err != nil {
		return fmt.Errorf("fleet enrollment response Node ID is invalid")
	}
	if strings.TrimSpace(r.CredentialID) == "" || len(r.CredentialID) > 128 || strings.TrimSpace(r.Credential) == "" {
		return fmt.Errorf("fleet enrollment response credential is invalid")
	}
	if r.CredentialScope != "opamp.connect" {
		return fmt.Errorf("fleet enrollment response credential scope is not opamp.connect")
	}
	endpoint, err := url.Parse(strings.TrimSpace(r.OpAMPEndpoint))
	if err != nil {
		return fmt.Errorf("fleet enrollment response OpAMP endpoint is invalid")
	}
	secureEndpoint := endpoint.Scheme == "wss" || endpoint.Scheme == "ws" && isLoopback(endpoint.Hostname())
	if endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || !secureEndpoint {
		return fmt.Errorf("fleet enrollment response OpAMP endpoint is invalid")
	}
	if r.CredentialExpiresAt != nil && !r.CredentialExpiresAt.After(time.Now()) {
		return fmt.Errorf("fleet enrollment response credential is expired")
	}
	return nil
}

func isLoopback(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

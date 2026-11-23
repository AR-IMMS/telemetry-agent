package domain

// AttemptStatus records the durable state of one Fleet exchange.
type AttemptStatus string

const (
	AttemptPending    AttemptStatus = "pending"
	AttemptRegistered AttemptStatus = "registered"
	AttemptFailed     AttemptStatus = "failed"
)

// Attempt stores immutable non-secret enrollment state and protected secret references.
type Attempt struct {
	AttemptID        string        `json:"attempt_id"`
	AgentID          string        `json:"agent_id"`
	Origin           string        `json:"fleet_origin"`
	InvitationRef    string        `json:"invitation_ref"`
	RecoveryProofRef string        `json:"recovery_proof_ref"`
	CredentialRef    string        `json:"credential_ref,omitempty"`
	Status           AttemptStatus `json:"status"`
	FailureCode      string        `json:"failure_code,omitempty"`
	Result           *Result       `json:"result,omitempty"`
}

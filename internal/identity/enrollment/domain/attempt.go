package domain

// AttemptStatus records the durable state of one enrollment operation.
type AttemptStatus string

const (
	// AttemptPending means retries must reuse this attempt ID and payload.
	AttemptPending AttemptStatus = "pending"
	// AttemptRegistered means Core binding and credential reference were committed.
	AttemptRegistered AttemptStatus = "registered"
	// AttemptFailed means Core returned a permanent error requiring operator action.
	AttemptFailed AttemptStatus = "failed"
)

// EnrollmentResult contains non-secret registration and control-plane metadata.
type EnrollmentResult struct {
	EnrollmentID   string       `json:"enrollment_id"`
	InstallationID string       `json:"installation_id"`
	Binding        Binding      `json:"binding"`
	ControlPlane   ControlPlane `json:"control_plane"`
}

// Attempt is the durable, secret-free representation of one operation.
type Attempt struct {
	Payload                     EnrollmentPayload `json:"payload"`
	CoreOrigin                  string            `json:"core_origin"`
	BootstrapCredentialRef      string            `json:"bootstrap_credential_ref"`
	AccessCredentialRef         string            `json:"access_credential_ref,omitempty"`
	Status                      AttemptStatus     `json:"status"`
	FailureCode                 string            `json:"failure_code,omitempty"`
	Result                      *EnrollmentResult `json:"result,omitempty"`
	PreviousResult              *EnrollmentResult `json:"previous_result,omitempty"`
	PreviousAccessCredentialRef string            `json:"previous_access_credential_ref,omitempty"`
}

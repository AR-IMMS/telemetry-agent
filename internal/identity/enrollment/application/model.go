package application

import (
	"fmt"
	"strings"

	identitydomain "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/domain"
)

// CoreOrigin is the trusted HTTPS origin accepted by the enrollment domain.
type CoreOrigin = identitydomain.CoreOrigin

// EnrollmentPayload is the immutable, non-secret enrollment input.
type EnrollmentPayload = identitydomain.EnrollmentPayload

// Installation identifies the durable Agent installation.
type Installation = identitydomain.Installation

// Agent describes the software version performing enrollment.
type Agent = identitydomain.Agent

// Environment describes the Agent operating system and architecture.
type Environment = identitydomain.Environment

// EnrollmentRequest is the secret-bearing input passed to the Core client port.
// Protocol serialization belongs to the Core API adapter.
type EnrollmentRequest struct {
	BootstrapToken string
	EnrollmentPayload
}

// EnrollmentResponse is the Core result validated before persistence.
type EnrollmentResponse struct {
	EnrollmentID   string
	InstallationID string
	Binding        Binding
	Credential     Credential
	ControlPlane   ControlPlane
}

// Binding records the active Core node binding.
type Binding = identitydomain.Binding

// Credential contains secret-bearing credential material from Core.
type Credential struct {
	Type        string
	AccessToken string
}

// ControlPlane identifies the accepted Core endpoint and polling interval.
type ControlPlane = identitydomain.ControlPlane

// Validate checks Core-issued data before any binding or credential is persisted.
func (r EnrollmentResponse) Validate(installationID string, trusted CoreOrigin) error {
	if err := identitydomain.ValidateBinding(r.EnrollmentID, r.InstallationID, r.Binding, installationID); err != nil {
		return err
	}
	if r.Credential.Type != "bearer" || strings.TrimSpace(r.Credential.AccessToken) == "" {
		return fmt.Errorf("Core enrollment response has an invalid Agent credential")
	}
	if err := identitydomain.ValidateControlPlane(r.ControlPlane, trusted); err != nil {
		return err
	}
	return nil
}

// AttemptStatus records the durable state of an enrollment operation.
type AttemptStatus = identitydomain.AttemptStatus

// EnrollmentResult contains non-secret registration and control-plane metadata.
type EnrollmentResult = identitydomain.EnrollmentResult

// Attempt is the durable, secret-free representation of one operation.
type Attempt = identitydomain.Attempt

const (
	// AttemptPending means retries must reuse this attempt ID and payload.
	AttemptPending = identitydomain.AttemptPending
	// AttemptRegistered means Core binding and credential reference were committed.
	AttemptRegistered = identitydomain.AttemptRegistered
	// AttemptFailed means Core returned a permanent error requiring operator action.
	AttemptFailed = identitydomain.AttemptFailed
)

// ParseCoreOrigin delegates to the canonical enrollment domain parser.
func ParseCoreOrigin(raw string) (CoreOrigin, error) {
	return identitydomain.ParseCoreOrigin(raw)
}

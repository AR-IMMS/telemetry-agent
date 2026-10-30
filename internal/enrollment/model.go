package enrollment

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var (
	// ErrUntrustedCoreOrigin indicates that a URL is not within the configured
	// HTTPS Core origin and must not receive an enrollment credential.
	ErrUntrustedCoreOrigin = errors.New("untrusted Core origin")
)

// CoreOrigin is the configured HTTPS origin trusted for enrollment traffic.
type CoreOrigin struct {
	value string
}

// ParseCoreOrigin accepts an HTTPS origin without credentials, path, query,
// or fragment. Requiring an origin keeps endpoint construction and trust checks
// deterministic.
func ParseCoreOrigin(raw string) (CoreOrigin, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return CoreOrigin{}, fmt.Errorf("parse Core origin: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return CoreOrigin{}, fmt.Errorf("Core URL must be an HTTPS origin without credentials or path")
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return CoreOrigin{}, fmt.Errorf("Core origin has invalid port")
		}
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.User = nil
	return CoreOrigin{value: strings.TrimSuffix(parsed.String(), "/")}, nil
}

// String returns the normalized HTTPS origin.
func (o CoreOrigin) String() string { return o.value }

// EnrollmentPayload contains the immutable, non-secret portion of one request.
// The bootstrap token is resolved from CredentialStore only when sending.
type EnrollmentPayload struct {
	AttemptID    string         `json:"enrollment_attempt_id"`
	Installation Installation   `json:"installation"`
	Agent        Agent          `json:"agent"`
	Environment  Environment    `json:"environment"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Installation identifies the durable Agent installation.
type Installation struct {
	ID string `json:"id"`
}

// Agent describes the software version performing enrollment.
type Agent struct {
	Version string `json:"version"`
}

// Environment describes the operating system and architecture of the Agent.
type Environment struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Hostname     string `json:"hostname,omitempty"`
}

// EnrollmentRequest is the wire request. BootstrapToken is secret and must
// never be persisted in SQLite or included in logs.
type EnrollmentRequest struct {
	BootstrapToken string `json:"bootstrap_token"`
	EnrollmentPayload
}

// EnrollmentResponse is the validated data returned by Core on success.
type EnrollmentResponse struct {
	EnrollmentID   string       `json:"enrollment_id"`
	InstallationID string       `json:"installation_id"`
	Binding        Binding      `json:"binding"`
	Credential     Credential   `json:"credential"`
	ControlPlane   ControlPlane `json:"control_plane"`
}

// Binding records the active Core node binding created by enrollment.
type Binding struct {
	ID     string `json:"id"`
	NodeID string `json:"node_id"`
	Status string `json:"status"`
}

// Credential contains the issued Agent credential and is secret-bearing.
type Credential struct {
	Type        string `json:"type"`
	AccessToken string `json:"access_token"`
}

// ControlPlane identifies the Core endpoint and polling interval returned by
// the enrollment service. The URL must remain on the configured Core origin.
type ControlPlane struct {
	BaseURL             string `json:"base_url"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

// Validate checks the Core response against the installation and trusted
// origin before any binding or credential is persisted.
func (r EnrollmentResponse) Validate(installationID string, trusted CoreOrigin) error {
	if strings.TrimSpace(r.EnrollmentID) == "" || strings.TrimSpace(r.Binding.ID) == "" ||
		strings.TrimSpace(r.Binding.NodeID) == "" {
		return fmt.Errorf("Core enrollment response is missing an identifier")
	}
	if r.InstallationID != installationID {
		return fmt.Errorf("Core enrollment response installation ID does not match this Agent")
	}
	if r.Binding.Status != "active" {
		return fmt.Errorf("Core enrollment response binding is not active")
	}
	if r.Credential.Type != "bearer" || strings.TrimSpace(r.Credential.AccessToken) == "" {
		return fmt.Errorf("Core enrollment response has an invalid Agent credential")
	}
	controlPlane, err := url.Parse(strings.TrimSpace(r.ControlPlane.BaseURL))
	if err != nil || !strings.EqualFold(controlPlane.Scheme, "https") || controlPlane.Hostname() == "" ||
		controlPlane.User != nil || controlPlane.RawQuery != "" || controlPlane.Fragment != "" {
		return fmt.Errorf("Core enrollment response has an invalid control-plane URL")
	}
	if !sameOrigin(trusted.value, controlPlane) {
		return ErrUntrustedCoreOrigin
	}
	if r.ControlPlane.PollIntervalSeconds <= 0 {
		return fmt.Errorf("Core enrollment response poll interval must be positive")
	}
	return nil
}

func sameOrigin(trusted string, candidate *url.URL) bool {
	base, err := url.Parse(trusted)
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Scheme, candidate.Scheme) &&
		strings.EqualFold(base.Hostname(), candidate.Hostname()) && effectivePort(base) == effectivePort(candidate)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}

// Validate checks required request fields and bounds metadata to the server
// contract's 4 KiB limit. Metadata contents remain the caller's responsibility.
func (p EnrollmentPayload) Validate() error {
	if strings.TrimSpace(p.AttemptID) == "" || strings.TrimSpace(p.Installation.ID) == "" ||
		strings.TrimSpace(p.Agent.Version) == "" {
		return fmt.Errorf("enrollment attempt, installation ID, and Agent version are required")
	}
	switch p.Environment.OS {
	case "linux", "windows", "darwin", "other":
	default:
		return fmt.Errorf("unsupported enrollment operating system %q", p.Environment.OS)
	}
	switch p.Environment.Architecture {
	case "amd64", "arm64", "386", "other":
	default:
		return fmt.Errorf("unsupported enrollment architecture %q", p.Environment.Architecture)
	}
	encoded, err := json.Marshal(p.Metadata)
	if err != nil {
		return fmt.Errorf("encode enrollment metadata: %w", err)
	}
	if len(encoded) > 4*1024 {
		return fmt.Errorf("enrollment metadata exceeds 4 KiB")
	}
	return nil
}

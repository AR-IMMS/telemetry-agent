package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation"
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

// Binding records the active Core node binding created by enrollment.
type Binding struct {
	ID     string `json:"id"`
	NodeID string `json:"node_id"`
	Status string `json:"status"`
}

// ControlPlane identifies the Core endpoint and polling interval returned by
// the enrollment service. The URL must remain on the configured Core origin.
type ControlPlane struct {
	BaseURL             string `json:"base_url"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

// ValidateBinding checks Core-issued identity against the local installation.
func ValidateBinding(enrollmentID, installationID string, binding Binding, expectedInstallationID string) error {
	for name, value := range map[string]string{
		"enrollment ID":   enrollmentID,
		"installation ID": installationID,
		"binding ID":      binding.ID,
		"binding Node ID": binding.NodeID,
	} {
		if _, err := installationid.ParseInstallationID(value); err != nil {
			return fmt.Errorf("Core enrollment response %s is invalid: %w", name, err)
		}
	}
	if installationID != expectedInstallationID {
		return fmt.Errorf("Core enrollment response installation ID does not match this Agent")
	}
	if binding.Status != "active" {
		return fmt.Errorf("Core enrollment response binding is not active")
	}
	return nil
}

// ValidateControlPlane ensures Core's endpoint remains under the trusted origin.
func ValidateControlPlane(value ControlPlane, trusted CoreOrigin) error {
	controlPlane, err := url.Parse(strings.TrimSpace(value.BaseURL))
	if err != nil || !strings.EqualFold(controlPlane.Scheme, "https") || controlPlane.Hostname() == "" ||
		controlPlane.User != nil || controlPlane.RawQuery != "" || controlPlane.Fragment != "" {
		return fmt.Errorf("Core enrollment response has an invalid control-plane URL")
	}
	if !sameOrigin(trusted.value, controlPlane) {
		return ErrUntrustedCoreOrigin
	}
	if value.PollIntervalSeconds <= 0 {
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
	if _, err := installationid.ParseInstallationID(p.AttemptID); err != nil {
		return fmt.Errorf("enrollment attempt ID is invalid: %w", err)
	}
	if _, err := installationid.ParseInstallationID(p.Installation.ID); err != nil {
		return fmt.Errorf("installation ID is invalid: %w", err)
	}
	if strings.TrimSpace(p.Agent.Version) == "" || len(p.Agent.Version) > 64 {
		return fmt.Errorf("Agent version must contain 1 to 64 characters")
	}
	if p.Environment.Hostname != "" && (len(p.Environment.Hostname) > 255 || strings.TrimSpace(p.Environment.Hostname) == "") {
		return fmt.Errorf("hostname must contain 1 to 255 characters when provided")
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

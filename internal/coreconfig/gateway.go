package coreconfig

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

var (
	// ErrStaleRevision means Core supplied a revision below the accepted one.
	ErrStaleRevision = errors.New("stale Core Gateway revision")
	// ErrRevisionConflict means Core reused one revision for different content.
	ErrRevisionConflict = errors.New("conflicting Core Gateway revision")
)

// GatewaySnapshot is a complete, one-setting Core desired snapshot. Callers
// must authenticate the source before passing it to the acceptance repository.
type GatewaySnapshot struct {
	InstallationID string
	Revision       uint64
	Endpoint       string
}

// GatewayOwner identifies the authority for the effective Gateway endpoint.
type GatewayOwner string

const (
	// BootstrapOwner applies before a Core snapshot is durably accepted.
	BootstrapOwner GatewayOwner = "bootstrap"
	// CoreOwner applies from the first committed Core snapshot onward.
	CoreOwner GatewayOwner = "core"
)

// EffectiveGateway identifies the desired endpoint and its local generation.
// Revision is zero while Bootstrap owns the endpoint.
type EffectiveGateway struct {
	Owner      GatewayOwner
	Endpoint   string
	Revision   uint64
	Generation uint64
}

// RuntimeGateway is the endpoint paired with a local activation generation.
// Revision is zero while the Bootstrap endpoint is effective.
type RuntimeGateway struct {
	Endpoint        string
	Revision        uint64
	Generation      uint64
	AppliedRevision uint64
	NeedsReadiness  bool
}

// Acceptance describes the committed revision-to-generation association.
// Changed indicates whether the effective endpoint changed in this call.
type Acceptance struct {
	Revision   uint64
	Generation uint64
	Changed    bool
}

// GatewayRepository is the narrow desired-state port used by a future
// authenticated ingress. Acceptance must be atomic with generation updates.
type GatewayRepository interface {
	// AcceptGatewaySnapshot atomically stores a validated, trusted snapshot.
	AcceptGatewaySnapshot(GatewaySnapshot) (Acceptance, error)
	// EffectiveGateway reads the Bootstrap or committed Core endpoint.
	EffectiveGateway() (EffectiveGateway, error)
}

// GatewayRuntimeRepository resolves the endpoint for a candidate generation
// and commits the endpoint only after that generation passes readiness.
type GatewayRuntimeRepository interface {
	GatewayRepository
	// GatewayForGeneration returns the candidate endpoint or the last applied endpoint.
	GatewayForGeneration(uint64) (RuntimeGateway, error)
	// MarkGatewayActivated associates the accepted Core revision with a validated local generation.
	MarkGatewayActivated(uint64) error
	// MarkGatewayApplied atomically records the local applied generation and endpoint.
	MarkGatewayApplied(uint64, string) error
}

// ValidateSnapshot rejects malformed identity, revision, or Gateway address.
// This is local schema validation, not proof of Core authentication.
func ValidateSnapshot(snapshot GatewaySnapshot) error {
	if _, err := identity.ParseInstallationID(snapshot.InstallationID); err != nil {
		return fmt.Errorf("validate Core Gateway installation ID: %w", err)
	}
	if snapshot.Revision == 0 || snapshot.Revision > math.MaxInt64 {
		return fmt.Errorf("Core Gateway revision must be a positive SQLite integer")
	}
	return ValidateGatewayEndpoint(snapshot.Endpoint)
}

// ValidateGatewayEndpoint accepts a Collector OTLP host:port, without URL
// scheme, credentials, path, or whitespace. It does not resolve DNS.
func ValidateGatewayEndpoint(endpoint string) error {
	if endpoint == "" || strings.ContainsAny(endpoint, " \t\r\n/@?#") {
		return fmt.Errorf("Gateway endpoint must be a host:port without URL syntax")
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" || port == "" {
		return fmt.Errorf("Gateway endpoint must be a valid host:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("Gateway endpoint port must be between 1 and 65535")
	}
	return nil
}

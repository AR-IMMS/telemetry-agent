package application

import (
	"errors"
	"fmt"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
)

// CandidateAdmissionRequest binds a compiled candidate to the current durable
// desired-state revision, Runtime generation, and verified baseline before
// activation begins.
type CandidateAdmissionRequest struct {
	DesiredSetRevision  uint64
	ExpectedGeneration  uint64
	ExpectedFingerprint string
	VerifiedFingerprint string
	Mapping             activationdomain.CandidateMapping
	RecoveryPath        string
}

// AdmissionResult identifies the reserved generation or an existing applied
// generation for an idempotent candidate request.
type AdmissionResult struct {
	Generation     uint64
	AlreadyApplied bool
}

// ErrStaleCandidate identifies candidate inputs that changed before admission.
var ErrStaleCandidate = errors.New("candidate is stale")

// ErrStaleDesiredRevision indicates that the submitted desired-state revision
// is no longer current.
var ErrStaleDesiredRevision = fmt.Errorf("%w: desired Integration revision changed", ErrStaleCandidate)

// ErrStaleM2Generation indicates that Runtime desired configuration changed
// before candidate admission.
var ErrStaleM2Generation = fmt.Errorf("%w: M2 desired generation changed", ErrStaleCandidate)

// ErrStaleBaseline indicates that ordered baseline inputs changed after
// candidate compilation.
var ErrStaleBaseline = fmt.Errorf("%w: clean baseline changed", ErrStaleCandidate)

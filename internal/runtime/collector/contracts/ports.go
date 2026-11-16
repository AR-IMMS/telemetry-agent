package contracts

import (
	"context"
	"errors"

	identity "github.com/ar-imms/telemetry-agent/internal/identity/host"
)

// ErrStaleCandidate reports that the desired state or baseline changed before candidate admission.
var ErrStaleCandidate = errors.New("Collector candidate inputs are stale")

// ErrInvalidCandidate marks a candidate rejected by Runtime compilation or pinned-binary validation.
var ErrInvalidCandidate = errors.New("invalid Collector candidate")

// BaselineSnapshot is the clean Collector render input and freshness identity
// consumed by reconciliation. It excludes the generated Integration candidate
// and lifecycle readiness overlay.
type BaselineSnapshot struct {
	Input             RenderInput
	DesiredGeneration uint64
	Fingerprint       string
}

// Layer identifies one ordered Runtime-owned Collector YAML input file.
type Layer struct {
	Name string
	Path string
}

// InlineLayer identifies an in-memory Collector document composed with file-backed layers.
type InlineLayer struct {
	Name     string
	Document map[string]any
}

// RenderInput is the cross-context request for a deterministic Collector configuration candidate.
// Configuration composes this value; Runtime owns its YAML interpretation and execution.
type RenderInput struct {
	Platform     identity.PlatformInfo
	Layers       []Layer
	InlineLayers []InlineLayer
	Values       map[string]string
}

// BaselineReader returns a clean baseline render input with the generation and
// fingerprint observed while building it. It does not promise a cross-store
// read transaction; CandidateApplier rechecks freshness before admission.
type BaselineReader interface {
	Snapshot() (BaselineSnapshot, error)
}

// CandidateApplyRequest binds one immutable compiled candidate to the desired
// Integration revision and clean M2 baseline against which it was compiled.
type CandidateApplyRequest struct {
	DesiredSetRevision  uint64
	ExpectedGeneration  uint64
	BaselineFingerprint string
	Candidate           Candidate
}

// CandidateActivationOutcome reports the lifecycle stage reached by one
// admitted candidate. Activated is distinct from readiness-applied state.
type CandidateActivationOutcome struct {
	Generation            uint64
	CandidateDigest       string
	EffectiveConfigDigest string
	Stage                 string
	Result                string
	RecoveryResult        string
	AlreadyApplied        bool
}

// CandidateApplier applies a complete candidate against its revision-bound
// baseline. A successful new admission can return before Collector readiness;
// AlreadyApplied is reserved for an idempotently confirmed applied generation.
type CandidateApplier interface {
	ApplyCandidate(context.Context, CandidateApplyRequest) (CandidateActivationOutcome, error)
}

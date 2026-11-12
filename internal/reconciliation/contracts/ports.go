package contracts

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
)

// BaselineSnapshot is the clean Collector render input and freshness identity
// consumed by reconciliation. It excludes the generated Integration candidate
// and lifecycle readiness overlay.
type BaselineSnapshot struct {
	Input             config.RenderInput
	DesiredGeneration uint64
	Fingerprint       string
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
	Candidate           collectorruntime.Candidate
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

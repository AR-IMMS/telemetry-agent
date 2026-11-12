package reconciliation

import "github.com/ar-imms/telemetry-agent/internal/reconciliation/contracts"

// BaselineSnapshot is the clean Collector render input and freshness identity
// consumed by reconciliation. It excludes the generated Integration candidate
// and lifecycle readiness overlay.
type BaselineSnapshot = contracts.BaselineSnapshot

// BaselineReader returns a clean baseline render input with its observed
// generation and fingerprint. Runtime rechecks freshness before admission.
type BaselineReader = contracts.BaselineReader

// CandidateApplyRequest binds one immutable compiled candidate to the desired
// Integration revision and clean M2 baseline against which it was compiled.
type CandidateApplyRequest = contracts.CandidateApplyRequest

// CandidateActivationOutcome reports the lifecycle stage reached by one
// admitted candidate. Activated is distinct from readiness-applied state.
type CandidateActivationOutcome = contracts.CandidateActivationOutcome

// CandidateApplier applies a complete candidate against its revision-bound
// baseline. A successful new admission can return before Collector readiness;
// AlreadyApplied is reserved for an idempotently confirmed applied generation.
type CandidateApplier = contracts.CandidateApplier

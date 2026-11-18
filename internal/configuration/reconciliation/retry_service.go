package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"time"

	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
)

var (
	// ErrRetryTargetChanged means the latest desired or baseline identity differs from the recorded target.
	ErrRetryTargetChanged = errors.New("latest desired or baseline target changed; the new target has its own retry budget")
)

// DesiredSnapshotReader exposes the Configuration desired revision needed to validate an explicit retry.
type DesiredSnapshotReader interface {
	Snapshot() (desiredstateapp.Snapshot, error)
}

// RetryService validates that durable retry evidence still refers to the current desired and baseline revisions.
// It records retries only after that freshness check succeeds.
type RetryService struct {
	desired     DesiredSnapshotReader
	baseline    BaselineReader
	diagnostics reconciliationcontracts.Diagnostics
}

// NewRetryService composes the owning Configuration readers and retry diagnostics repository.
func NewRetryService(desired DesiredSnapshotReader, baseline BaselineReader, diagnostics reconciliationcontracts.Diagnostics) RetryService {
	return RetryService{desired: desired, baseline: baseline, diagnostics: diagnostics}
}

// RetryCurrentTarget schedules the latest pending target and returns its desired revision.
func (s RetryService) RetryCurrentTarget(ctx context.Context) (uint64, error) {
	if ctx == nil || s.desired == nil || s.baseline == nil || s.diagnostics == nil {
		return 0, fmt.Errorf("retry context and Configuration dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	evidence, err := s.diagnostics.Evidence()
	if err != nil {
		return 0, err
	}
	desired, err := s.desired.Snapshot()
	if err != nil {
		return 0, err
	}
	baseline, err := s.baseline.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	if !TargetIsCurrent(evidence, desired.Revision, baseline) {
		return 0, ErrRetryTargetChanged
	}
	if err := s.diagnostics.RetryTarget(evidence.TargetKey, time.Now().UTC()); err != nil {
		return 0, err
	}
	return evidence.Target.DesiredRevision, nil
}

// TargetIsCurrent reports whether evidence matches the latest desired revision and clean Runtime baseline.
func TargetIsCurrent(evidence reconciliationcontracts.TargetEvidence, desiredRevision uint64, baseline BaselineSnapshot) bool {
	return evidence.Target.DesiredRevision == desiredRevision &&
		evidence.Target.DesiredGeneration == baseline.DesiredGeneration &&
		evidence.Target.BaselineFingerprint == baseline.Fingerprint
}

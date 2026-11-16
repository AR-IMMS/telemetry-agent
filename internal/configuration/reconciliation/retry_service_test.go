package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
)

type retryServiceDesired struct{ snapshot configuration.DesiredSnapshot }

func (r retryServiceDesired) Snapshot() (configuration.DesiredSnapshot, error) {
	return r.snapshot, nil
}

type retryServiceBaseline struct{ snapshot BaselineSnapshot }

func (r retryServiceBaseline) Snapshot() (BaselineSnapshot, error) { return r.snapshot, nil }

type retryServiceDiagnostics struct {
	evidence   reconciliationcontracts.TargetEvidence
	retryKey   string
	retryAt    time.Time
	retryCalls int
	err        error
}

func (r *retryServiceDiagnostics) Evidence() (reconciliationcontracts.TargetEvidence, error) {
	return r.evidence, r.err
}
func (*retryServiceDiagnostics) History(int) ([]reconciliationcontracts.AttemptRecord, error) {
	return nil, nil
}
func (r *retryServiceDiagnostics) RetryTarget(key string, at time.Time) error {
	r.retryCalls++
	r.retryKey, r.retryAt = key, at
	return r.err
}

func TestRetryServiceRejectsStaleDesiredAndBaselineTarget(t *testing.T) {
	diagnostics := &retryServiceDiagnostics{evidence: reconciliationcontracts.TargetEvidence{
		Target: reconciliationcontracts.Target{DesiredRevision: 9, DesiredGeneration: 4, BaselineFingerprint: "old"}, TargetKey: "target-old",
	}}
	service := NewRetryService(
		retryServiceDesired{snapshot: configuration.DesiredSnapshot{Revision: 10}},
		retryServiceBaseline{snapshot: BaselineSnapshot{DesiredGeneration: 4, Fingerprint: "current"}},
		diagnostics,
	)
	_, err := service.RetryCurrentTarget(context.Background())
	if !errors.Is(err, ErrRetryTargetChanged) {
		t.Fatalf("err=%v, want ErrRetryTargetChanged", err)
	}
	if diagnostics.retryCalls != 0 {
		t.Fatalf("retry calls=%d, want 0", diagnostics.retryCalls)
	}
}

func TestRetryServiceSchedulesOnlyCurrentTarget(t *testing.T) {
	diagnostics := &retryServiceDiagnostics{evidence: reconciliationcontracts.TargetEvidence{
		Target: reconciliationcontracts.Target{DesiredRevision: 10, DesiredGeneration: 4, BaselineFingerprint: "current"}, TargetKey: "target-current",
	}}
	service := NewRetryService(
		retryServiceDesired{snapshot: configuration.DesiredSnapshot{Revision: 10}},
		retryServiceBaseline{snapshot: BaselineSnapshot{DesiredGeneration: 4, Fingerprint: "current"}},
		diagnostics,
	)
	revision, err := service.RetryCurrentTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if revision != 10 || diagnostics.retryCalls != 1 || diagnostics.retryKey != "target-current" {
		t.Fatalf("revision=%d calls=%d key=%q", revision, diagnostics.retryCalls, diagnostics.retryKey)
	}
	if diagnostics.retryAt.IsZero() || diagnostics.retryAt.Location() != time.UTC {
		t.Fatalf("retry timestamp=%v, want UTC timestamp", diagnostics.retryAt)
	}
}

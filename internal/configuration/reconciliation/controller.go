package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
)

// Failure gives diagnostics a stable stage, code, and retry classification.
type Failure struct {
	Stage string
	Code  string
	Class FailureClass
	Err   error
}

func (f Failure) Error() string {
	if f.Err == nil {
		return f.Code
	}
	return f.Err.Error()
}
func (f Failure) Unwrap() error { return f.Err }

// Result reports what a reconciliation pass established without implying health or delivery.
type Result struct {
	DesiredRevision     uint64
	DesiredGeneration   uint64
	BaselineFingerprint string
	Status              Status
	Attempt             uint64
	CandidateDigest     string
	Generation          uint64
	Stale               bool
}

type desiredReader interface {
	Snapshot() (configuration.DesiredSnapshot, error)
}
type resolver interface {
	Resolve(integration.IntegrationInstance) (integration.ResolvedInstance, error)
}
type compiler interface {
	Compile(input contracts.RenderInput, resolved []integration.ResolvedInstance) (contracts.Candidate, error)
}

type candidateValidator func(context.Context, contracts.Candidate) error
type attemptRepository interface {
	BeginAttempt(Target) (Attempt, error)
	RecordCandidate(Attempt, string) error
	FinishAttempt(Attempt, Outcome) error
}
type candidateOutcomeReader interface {
	Lookup(uint64, string, string) (CandidateOutcome, uint64, bool, error)
}

// Controller performs one serialized desired-to-candidate convergence pass.
type Controller struct {
	Desired      desiredReader
	Baseline     BaselineReader
	Resolver     resolver
	Compiler     compiler
	Validator    candidateValidator
	Applier      CandidateApplier
	Attempts     attemptRepository
	Outcomes     candidateOutcomeReader
	PollInterval time.Duration
	mu           sync.Mutex
}

// Reconcile resolves the complete enabled snapshot and asks M2 to apply a
// candidate. An activated result stays Pending until M2 records terminal readiness.
func (c *Controller) Reconcile(ctx context.Context) (Result, error) {
	if ctx == nil || c == nil || c.Desired == nil || c.Baseline == nil || c.Resolver == nil || c.Compiler == nil || c.Validator == nil || c.Applier == nil || c.Attempts == nil {
		return Result{}, fmt.Errorf("reconciliation dependencies and context are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	snapshot, err := c.Desired.Snapshot()
	if err != nil {
		return Result{}, Failure{Stage: "snapshot", Code: "desired_snapshot_failed", Class: FailureTransient, Err: err}
	}
	baseline, err := c.Baseline.Snapshot(ctx)
	if err != nil {
		return Result{}, Failure{Stage: "baseline", Code: "baseline_snapshot_failed", Class: FailureTransient, Err: err}
	}
	target := Target{DesiredRevision: snapshot.Revision, DesiredGeneration: baseline.DesiredGeneration, BaselineFingerprint: baseline.Fingerprint}
	result := Result{DesiredRevision: target.DesiredRevision, DesiredGeneration: target.DesiredGeneration, BaselineFingerprint: target.BaselineFingerprint, Status: StatusPending}
	attempt, err := c.Attempts.BeginAttempt(target)
	if err != nil {
		if errors.Is(err, ErrTargetAlreadyApplied) {
			result.Status = StatusApplied
			return result, nil
		}
		if errors.Is(err, ErrAwaitingLifecycle) {
			return result, nil
		}
		return result, err
	}
	result.Attempt = attempt.Number
	finish := func(outcome Outcome, cause error) (Result, error) {
		if saveErr := c.Attempts.FinishAttempt(attempt, outcome); saveErr != nil {
			return result, errors.Join(cause, fmt.Errorf("persist reconciliation outcome: %w", saveErr))
		}
		return result, cause
	}
	resolved := make([]integration.ResolvedInstance, 0, len(snapshot.Instances))
	for _, instance := range snapshot.Instances {
		if !instance.Enabled {
			continue
		}
		value, resolveErr := c.Resolver.Resolve(instance)
		if resolveErr != nil {
			failure := classify("resolve", resolveErr)
			return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, SafeDetail: "integration resolution failed", RetryAt: retryAt(attempt.Number)}, failure)
		}
		resolved = append(resolved, value)
	}
	candidate, err := c.Compiler.Compile(baseline.Input, resolved)
	if err != nil {
		failure := classify("compile", err)
		return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, SafeDetail: "candidate compilation failed", RetryAt: retryAt(attempt.Number)}, failure)
	}
	result.CandidateDigest = candidate.Digest()
	if err := c.Attempts.RecordCandidate(attempt, result.CandidateDigest); err != nil {
		failure := classify("diagnostics", err)
		return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, CandidateDigest: result.CandidateDigest, SafeDetail: "candidate identity could not be persisted", RetryAt: retryAt(attempt.Number)}, failure)
	}
	if c.Outcomes != nil {
		mapping, generation, found, lookupErr := c.Outcomes.Lookup(snapshot.Revision, baseline.Fingerprint, candidate.Digest())
		if lookupErr != nil {
			failure := classify("journal", lookupErr)
			return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, SafeDetail: "M2 activation evidence could not be read", RetryAt: retryAt(attempt.Number)}, failure)
		}
		if found && mapping.Outcome == "applied" {
			result.Status = StatusApplied
			result.Generation = generation
			return finish(Outcome{Status: StatusApplied, Stage: "terminal", Code: "applied", CandidateDigest: candidate.Digest(), Generation: generation}, nil)
		}
		if found && mapping.Outcome == "activated" {
			result.Status = StatusPending
			result.Generation = generation
			return finish(Outcome{Status: StatusPending, Stage: "activation", Code: "awaiting_m2_terminal_outcome", CandidateDigest: candidate.Digest(), Generation: generation, SafeDetail: "activation is waiting for Collector readiness"}, nil)
		}
	}
	if err = c.Validator(ctx, candidate); err != nil {
		failure := classify("validate", err)
		return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, CandidateDigest: candidate.Digest(), SafeDetail: "candidate validation failed", RetryAt: retryAt(attempt.Number)}, failure)
	}
	activation, err := c.Applier.ApplyCandidate(ctx, CandidateApplyRequest{DesiredSetRevision: snapshot.Revision, ExpectedGeneration: baseline.DesiredGeneration, BaselineFingerprint: baseline.Fingerprint, Candidate: candidate})
	if err != nil {
		if errors.Is(err, ErrStalePlan) || errors.Is(err, contracts.ErrStaleCandidate) {
			result.Stale = true
			return finish(Outcome{Status: StatusPending, Stage: "admission", Code: "stale_plan", SafeDetail: "desired inputs changed; a later pass will read the latest snapshot"}, nil)
		}
		failure := classify("activation", err)
		return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, CandidateDigest: candidate.Digest(), Generation: activation.Generation, SafeDetail: "candidate lifecycle failed", RetryAt: retryAt(attempt.Number)}, failure)
	}
	result.Generation = activation.Generation
	if activation.AlreadyApplied {
		result.Status = StatusApplied
		return finish(Outcome{Status: StatusApplied, Stage: "terminal", Code: "applied", CandidateDigest: candidate.Digest(), Generation: activation.Generation}, nil)
	}
	if c.Outcomes != nil {
		mapping, generation, err := c.awaitTerminal(ctx, snapshot.Revision, baseline.Fingerprint, candidate.Digest())
		if err != nil {
			failure := classify("activation", err)
			if mapping.RecoveryResult == "failed" {
				failure = Failure{Stage: "recovery", Code: "lkg_recovery_failed", Class: FailurePermanent, Err: err}
			}
			result.Generation = generation
			return finish(Outcome{Status: attemptStatus(failure.Class, attempt.Number), Stage: failure.Stage, Code: failure.Code, Class: failure.Class, CandidateDigest: candidate.Digest(), Generation: generation, SafeDetail: "M2 terminal activation outcome recorded", RetryAt: retryAt(attempt.Number)}, failure)
		}
		result.Generation, result.Status = generation, StatusApplied
		return finish(Outcome{Status: StatusApplied, Stage: "terminal", Code: "applied", CandidateDigest: candidate.Digest(), Generation: generation}, nil)
	}
	return finish(Outcome{Status: StatusPending, Stage: "activation", Code: "awaiting_m2_terminal_outcome", CandidateDigest: candidate.Digest(), Generation: activation.Generation, SafeDetail: "candidate admitted; awaiting lifecycle readiness evidence"}, nil)
}

func (c *Controller) awaitTerminal(ctx context.Context, revision uint64, fingerprint, digest string) (CandidateOutcome, uint64, error) {
	interval := c.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		mapping, generation, found, err := c.Outcomes.Lookup(revision, fingerprint, digest)
		if err != nil {
			return mapping, generation, fmt.Errorf("read M2 candidate outcome: %w", err)
		}
		if found {
			switch mapping.Outcome {
			case "applied":
				return mapping, generation, nil
			case "failed", "interrupted":
				return mapping, generation, fmt.Errorf("M2 candidate ended with %s at %s", mapping.Outcome, mapping.Stage)
			}
		}
		select {
		case <-ctx.Done():
			return mapping, generation, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Run reconciles once at startup, then polls SQLite on every tick. The ticker
// is the cross-process change detector; local wakeups may call Reconcile directly.
func (c *Controller) Run(ctx context.Context, interval time.Duration) error {
	if c == nil || ctx == nil || interval <= 0 {
		return fmt.Errorf("reconciliation context and positive poll interval are required")
	}
	if _, err := c.reconcileLatest(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logPassFailure(err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := c.reconcileLatest(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logPassFailure(err)
			}
		}
	}
}

func logPassFailure(err error) {
	if errors.Is(err, ErrAttemptBlocked) || errors.Is(err, ErrAttemptBudgetExhausted) || errors.Is(err, ErrAttemptBackoff) || errors.Is(err, ErrTargetAlreadyApplied) || errors.Is(err, ErrAwaitingLifecycle) {
		return
	}
	var failure Failure
	if errors.As(err, &failure) {
		slog.Warn("Agent reconciliation pass failed", "stage", failure.Stage, "code", failure.Code, "class", failure.Class)
		return
	}
	slog.Error("Agent reconciliation pass failed", "stage", "controller", "code", "pass_failed")
}

func (c *Controller) reconcileLatest(ctx context.Context) (Result, error) {
	var result Result
	var err error
	for i := 0; i < 5; i++ {
		result, err = c.Reconcile(ctx)
		if err != nil {
			return result, err
		}
		if result.Stale {
			continue
		}
		if result.Status != StatusApplied {
			return result, nil
		}
		desired, de := c.Desired.Snapshot()
		if de != nil {
			return result, Failure{Stage: "latest_desired", Code: "desired_snapshot_failed", Class: FailureTransient, Err: de}
		}
		baseline, be := c.Baseline.Snapshot(ctx)
		if be != nil {
			return result, Failure{Stage: "latest_baseline", Code: "baseline_snapshot_failed", Class: FailureTransient, Err: be}
		}
		if desired.Revision == result.DesiredRevision && baseline.DesiredGeneration == result.DesiredGeneration && baseline.Fingerprint == result.BaselineFingerprint {
			return result, nil
		}
	}
	return result, nil
}

func classify(stage string, err error) Failure {
	var failure Failure
	if errors.As(err, &failure) {
		return failure
	}
	class := FailureTransient
	code := stage + "_failed"
	if errors.Is(err, integration.ErrInvalidInstance) || errors.Is(err, integration.ErrUnknownDefinition) || errors.Is(err, integration.ErrInvalidContribution) || errors.Is(err, contracts.ErrInvalidCandidate) {
		class = FailurePermanent
		code = "invalid_candidate"
	}
	if stage == "activation" {
		class = FailureActivation
		code = "activation_failed"
	}
	return Failure{Stage: stage, Code: code, Class: class, Err: err}
}
func attemptStatus(class FailureClass, number uint64) Status {
	if class == FailurePermanent {
		return StatusBlocked
	}
	if number >= MaxAttempts {
		return StatusNeedsAttention
	}
	return StatusPending
}
func retryAt(number uint64) time.Time {
	delay := time.Second
	for i := uint64(1); i < number && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	return time.Now().Add(delay)
}

// ErrStalePlan reports a candidate rejected before M2 admission because its inputs changed.
var ErrStalePlan = errors.New("reconciliation plan is stale")

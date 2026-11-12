package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation/contracts"
)

type candidateLifecycleRepository interface {
	AdmitCandidate(agentstate.CandidateAdmission) (agentstate.CandidateAdmissionResult, error)
	CompleteCandidateFailure(uint64, string, string, string) error
}

type candidateActivationPlan struct {
	Mapping     agentstate.CandidateMapping
	Content     []byte
	Environment []string
}

type candidatePlanBuilder func(agentstate.State) (candidateActivationPlan, error)
type candidateActivator func(context.Context, []byte, []string) error

// ApplyCandidate extends the existing configuration coordinator with
// revision-bound admission while sharing its process and OS lifecycle locks.
func (c configurationCoordinator) ApplyCandidate(
	ctx context.Context,
	request contracts.CandidateApplyRequest,
	build candidatePlanBuilder,
	activate candidateActivator,
	repository candidateLifecycleRepository,
) (contracts.CandidateActivationOutcome, error) {
	var result contracts.CandidateActivationOutcome
	err := c.withLifecycleLock(func() error {
		if ctx == nil || build == nil || activate == nil || repository == nil || c.prepare == nil {
			return fmt.Errorf("candidate lifecycle dependencies are required")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := c.store.Load()
		if err != nil {
			return fmt.Errorf("load Agent state before candidate admission: %w", err)
		}
		if state.DesiredGeneration != request.ExpectedGeneration {
			return agentstate.ErrStaleM2Generation
		}
		plan, err := build(state)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		recoveryPath, err := c.prepare(state)
		if err != nil {
			return fmt.Errorf("prepare candidate recovery artifact: %w", err)
		}
		admission, err := repository.AdmitCandidate(agentstate.CandidateAdmission{
			DesiredSetRevision:  request.DesiredSetRevision,
			ExpectedGeneration:  request.ExpectedGeneration,
			ExpectedFingerprint: request.BaselineFingerprint,
			VerifiedFingerprint: plan.Mapping.BaselineFingerprint,
			Mapping:             plan.Mapping,
			RecoveryPath:        recoveryPath,
		})
		if err != nil {
			if recoveryPath != "" {
				_ = os.Remove(recoveryPath)
			}
			return err
		}
		result = contracts.CandidateActivationOutcome{
			Generation:            admission.Generation,
			CandidateDigest:       plan.Mapping.CandidateDigest,
			EffectiveConfigDigest: plan.Mapping.EffectiveConfigDigest,
			Stage:                 "admission",
		}
		if admission.AlreadyApplied {
			result.Result, result.AlreadyApplied = "applied", true
			if recoveryPath != "" {
				_ = os.Remove(recoveryPath)
			}
			return nil
		}
		if err := activate(ctx, plan.Content, plan.Environment); err != nil {
			recoveryErr := c.recoverCandidate(recoveryPath)
			if recoveryErr != nil {
				result.Stage, result.Result, result.RecoveryResult = "recovery", "failed", "failed"
				return fmt.Errorf("candidate activation failed: %v; restore previous config: %w", err, recoveryErr)
			}
			stage := "activation"
			if errors.Is(err, bootstrap.ErrNativeConfigValidation) {
				stage = "final_validation"
			}
			if terminalErr := repository.CompleteCandidateFailure(admission.Generation, stage, "failed", "succeeded"); terminalErr != nil {
				return fmt.Errorf("candidate activation failed: %v; persist terminal outcome: %w", err, terminalErr)
			}
			if recoveryPath != "" {
				_ = os.Remove(recoveryPath)
			}
			result.Stage, result.Result, result.RecoveryResult = stage, "failed", "succeeded"
			return err
		}
		_, err = c.store.Update(func(current *agentstate.State) error {
			if current.PendingActivation == nil || current.PendingActivation.Generation != admission.Generation {
				return fmt.Errorf("candidate activation intent changed before activation commit")
			}
			return current.MarkCandidateGenerationActivated(admission.Generation)
		})
		if err != nil {
			recoveryErr := c.recoverCandidate(recoveryPath)
			if recoveryErr != nil {
				result.Stage, result.Result, result.RecoveryResult = "recovery", "failed", "failed"
				return fmt.Errorf("candidate state commit failed: %v; restore previous config: %w", err, recoveryErr)
			}
			if terminalErr := repository.CompleteCandidateFailure(admission.Generation, "state_commit", "failed", "succeeded"); terminalErr != nil {
				return fmt.Errorf("candidate state commit failed: %v; persist failure: %w", err, terminalErr)
			}
			if recoveryPath != "" {
				_ = os.Remove(recoveryPath)
			}
			return err
		}
		result.Stage, result.Result = "activation", "activated"
		return nil
	})
	return result, err
}

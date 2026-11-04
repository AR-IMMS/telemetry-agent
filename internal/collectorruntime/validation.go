package collectorruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
)

// ValidateCandidate invokes the pinned Collector validator without activation.
func ValidateCandidate(ctx context.Context, runner bootstrap.CommandRunner, binaryPath string, candidate Candidate, environment []string) error {
	if len(candidate.content) == 0 {
		return fmt.Errorf("%w: candidate is empty", ErrNativeValidation)
	}
	digest := sha256.Sum256(candidate.content)
	if candidate.Digest() != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("%w: candidate digest does not match its bytes", ErrNativeValidation)
	}
	artifact, err := bootstrap.SelectArtifact(candidate.Platform())
	if err != nil {
		return fmt.Errorf("%w: resolve pinned Collector artifact: %v", ErrNativeValidation, err)
	}
	if candidate.CollectorVersion() == "" || candidate.CollectorVersion() != artifact.Version {
		return fmt.Errorf("%w: candidate Collector version %q does not match pinned version %q", ErrNativeValidation, candidate.CollectorVersion(), artifact.Version)
	}
	if err := bootstrap.ValidateRenderedConfig(ctx, runner, binaryPath, candidate.Bytes(), environment); err != nil {
		// Collector diagnostics may echo configuration fragments. Keep the typed
		// failure useful without exposing candidate values to callers or logs.
		return fmt.Errorf("%w: pinned Collector rejected the candidate configuration", ErrNativeValidation)
	}
	return nil
}

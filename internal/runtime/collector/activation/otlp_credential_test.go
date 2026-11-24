package agentlifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	fleetdomain "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
)

func TestPrepareCollectorOTLPTokenRequiresCommittedScopedCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows protected-store integration requires a service-account test environment")
	}
	statePath := filepath.Join(t.TempDir(), "state.db")
	if path, err := PrepareCollectorOTLPToken(statePath, supervisorFleetAttemptRepository{err: fleetenrollment.ErrNoAttempt}); err != nil || path != "" {
		t.Fatalf("unenrolled token path=%q, err=%v", path, err)
	}
	attempt := fleetdomain.Attempt{Status: fleetdomain.AttemptRegistered, OTLPCredentialRef: "fleet/otlp/ref", Result: &fleetdomain.Result{OTLPCredentialID: "otlp-id", OTLPCredentialScope: "opamp.connect"}}
	if path, err := PrepareCollectorOTLPToken(statePath, supervisorFleetAttemptRepository{attempt: attempt}); err == nil || path != "" {
		t.Fatalf("wrong-scope token path=%q, err=%v", path, err)
	}
	attempt.Result.OTLPCredentialScope = "otlp.write"
	if path, err := PrepareCollectorOTLPToken(statePath, supervisorFleetAttemptRepository{attempt: attempt}); err == nil || path != "" {
		t.Fatalf("missing protected token path=%q, err=%v", path, err)
	}
	if err := credentialstore.New(statePath).Put(attempt.OTLPCredentialRef, "test-only-otlp-bearer"); err != nil {
		t.Fatal(err)
	}
	path, err := PrepareCollectorOTLPToken(statePath, supervisorFleetAttemptRepository{attempt: attempt})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "test-only-otlp-bearer" {
		t.Fatalf("materialized OTLP bearer invalid: %v", err)
	}
	if filepath.Base(path) != "otlp-collector.token" {
		t.Fatalf("unexpected Collector token path %q", path)
	}
}

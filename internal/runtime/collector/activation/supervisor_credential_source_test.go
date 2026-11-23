package agentlifecycle

import (
	"testing"

	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	coredomain "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/domain"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	fleetdomain "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
)

func TestFleetSupervisorCredentialFailsClosedWhenFleetAttemptIsMissing(t *testing.T) {
	coreRepository := supervisorCoreAttemptRepository{attempt: coredomain.Attempt{
		Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref",
	}}
	fleetRepository := supervisorFleetAttemptRepository{err: fleetenrollment.ErrNoAttempt}
	supervisor := opampdomain.SupervisorContext{
		CredentialSource: opampdomain.CredentialSourceFleet,
		OpAMPServerURL:   "wss://fleet.example.test/v1/opamp",
	}

	credential, err := currentSupervisorCredentialReference(coreRepository, fleetRepository, supervisor)
	if err == nil || credential != "" {
		t.Fatalf("Fleet endpoint without Fleet enrollment state returned credential %q, err=%v", credential, err)
	}
}

func TestFleetSupervisorCredentialFailsClosedWhenFleetEndpointMismatches(t *testing.T) {
	coreRepository := supervisorCoreAttemptRepository{attempt: coredomain.Attempt{
		Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref",
	}}
	fleetRepository := supervisorFleetAttemptRepository{attempt: fleetdomain.Attempt{
		Status:        fleetdomain.AttemptRegistered,
		CredentialRef: "fleet-credential-ref",
		Result:        &fleetdomain.Result{OpAMPEndpoint: "wss://different-fleet.example.test/v1/opamp"},
	}}
	supervisor := opampdomain.SupervisorContext{
		CredentialSource: opampdomain.CredentialSourceFleet,
		OpAMPServerURL:   "wss://fleet.example.test/v1/opamp",
	}

	credential, err := currentSupervisorCredentialReference(coreRepository, fleetRepository, supervisor)
	if err == nil || credential != "" {
		t.Fatalf("mismatched Fleet enrollment returned credential %q, err=%v", credential, err)
	}
}

func TestLegacyRemoteSupervisorEndpointFailsClosedWithoutFleetState(t *testing.T) {
	coreRepository := supervisorCoreAttemptRepository{attempt: coredomain.Attempt{
		Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref",
	}}
	fleetRepository := supervisorFleetAttemptRepository{err: fleetenrollment.ErrNoAttempt}
	supervisor := opampdomain.SupervisorContext{OpAMPServerURL: "wss://fleet.example.test/v1/opamp"}

	credential, err := currentSupervisorCredentialReference(coreRepository, fleetRepository, supervisor)
	if err == nil || credential != "" {
		t.Fatalf("unattributed remote endpoint returned credential %q, err=%v", credential, err)
	}
}

func TestLegacyRemoteSupervisorEndpointFailsClosedWhenFleetAttemptMismatches(t *testing.T) {
	coreRepository := supervisorCoreAttemptRepository{attempt: coredomain.Attempt{
		Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref",
	}}
	fleetRepository := supervisorFleetAttemptRepository{attempt: fleetdomain.Attempt{
		Status:        fleetdomain.AttemptRegistered,
		CredentialRef: "fleet-credential-ref",
		Result:        &fleetdomain.Result{OpAMPEndpoint: "wss://different-fleet.example.test/v1/opamp"},
	}}
	supervisor := opampdomain.SupervisorContext{OpAMPServerURL: "wss://fleet.example.test/v1/opamp"}

	credential, err := currentSupervisorCredentialReference(coreRepository, fleetRepository, supervisor)
	if err == nil || credential != "" {
		t.Fatalf("remote endpoint with mismatched Fleet state returned credential %q, err=%v", credential, err)
	}
}

func TestCoreSupervisorCredentialFailsClosedWhenFleetStateOwnsEndpoint(t *testing.T) {
	coreRepository := supervisorCoreAttemptRepository{attempt: coredomain.Attempt{
		Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref",
	}}
	fleetRepository := supervisorFleetAttemptRepository{attempt: fleetdomain.Attempt{
		Status:        fleetdomain.AttemptRegistered,
		CredentialRef: "fleet-credential-ref",
		Result:        &fleetdomain.Result{OpAMPEndpoint: "wss://fleet.example.test/v1/opamp"},
	}}
	supervisor := opampdomain.SupervisorContext{
		CredentialSource: opampdomain.CredentialSourceCore,
		OpAMPServerURL:   "wss://fleet.example.test/v1/opamp",
	}

	credential, err := currentSupervisorCredentialReference(coreRepository, fleetRepository, supervisor)
	if err == nil || credential != "" {
		t.Fatalf("Core source for a registered Fleet endpoint returned credential %q, err=%v", credential, err)
	}
}

func TestSupervisorCredentialUsesTheConfiguredIssuer(t *testing.T) {
	tests := []struct {
		name    string
		source  opampdomain.CredentialSource
		core    coredomain.Attempt
		fleet   fleetdomain.Attempt
		want    string
		wantErr bool
	}{
		{
			name:   "Core source uses Core credential",
			source: opampdomain.CredentialSourceCore,
			core:   coredomain.Attempt{Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref"},
			want:   "core-credential-ref",
		},
		{
			name:   "Fleet source uses matching Fleet credential",
			source: opampdomain.CredentialSourceFleet,
			core:   coredomain.Attempt{Status: coredomain.AttemptRegistered, AccessCredentialRef: "core-credential-ref"},
			fleet:  fleetdomain.Attempt{Status: fleetdomain.AttemptRegistered, CredentialRef: "fleet-credential-ref", Result: &fleetdomain.Result{OpAMPEndpoint: "wss://fleet.example.test/v1/opamp"}},
			want:   "fleet-credential-ref",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint := "ws://127.0.0.1:4320/v1/opamp"
			if test.source == opampdomain.CredentialSourceFleet {
				endpoint = "wss://fleet.example.test/v1/opamp"
			}
			fleetRepository := supervisorFleetAttemptRepository{attempt: test.fleet}
			if test.source != opampdomain.CredentialSourceFleet {
				fleetRepository.err = fleetenrollment.ErrNoAttempt
			}
			credential, err := currentSupervisorCredentialReference(
				supervisorCoreAttemptRepository{attempt: test.core},
				fleetRepository,
				opampdomain.SupervisorContext{CredentialSource: test.source, OpAMPServerURL: endpoint},
			)
			if (err != nil) != test.wantErr || credential != test.want {
				t.Fatalf("credential=%q err=%v, want credential=%q err=%t", credential, err, test.want, test.wantErr)
			}
		})
	}
}

type supervisorCoreAttemptRepository struct {
	enrollment.AttemptRepository
	attempt coredomain.Attempt
	err     error
}

func (r supervisorCoreAttemptRepository) LoadAttempt() (coredomain.Attempt, error) {
	return r.attempt, r.err
}

type supervisorFleetAttemptRepository struct {
	fleetenrollment.AttemptRepository
	attempt fleetdomain.Attempt
	err     error
}

func (r supervisorFleetAttemptRepository) LoadAttempt() (fleetdomain.Attempt, error) {
	if r.err != nil {
		return fleetdomain.Attempt{}, r.err
	}
	return r.attempt, nil
}

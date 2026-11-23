package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	fleetdomain "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
)

func TestRunEnrollmentUsesHiddenTokenProviderAndDoesNotPrintCredentials(t *testing.T) {
	var gotState, gotOrigin, gotToken string
	deps := dependencies{
		readBootstrapToken: func(_ io.Writer) (string, error) { return "bootstrap-secret", nil },
		runEnrollment: func(_ context.Context, statePath, coreOrigin, token string, resume, reEnroll bool) (enrollment.EnrollmentResult, error) {
			gotState, gotOrigin, gotToken = statePath, coreOrigin, token
			if resume || reEnroll {
				t.Fatal("unexpected resume mode")
			}
			return enrollment.EnrollmentResult{
				EnrollmentID: "enrollment-1", InstallationID: "installation-1",
				Binding:      enrollment.Binding{ID: "binding-1", NodeID: "node-1", Status: "active"},
				ControlPlane: enrollment.ControlPlane{BaseURL: "https://core.example/api", PollIntervalSeconds: 30},
			}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), []string{"--core-origin", "https://core.example", "--state-path", "state.db"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("runEnrollment() = %d, stderr=%q", code, stderr.String())
	}
	if gotState != "state.db" || gotOrigin != "https://core.example" || gotToken != "bootstrap-secret" {
		t.Fatalf("runner args = %q %q %q", gotState, gotOrigin, gotToken)
	}
	if strings.Contains(stdout.String()+stderr.String(), "bootstrap-secret") || strings.Contains(stdout.String()+stderr.String(), "agent-secret") {
		t.Fatal("enrollment command printed credential material")
	}
}

func TestRunEnrollmentResumeDoesNotPromptForToken(t *testing.T) {
	read := false
	deps := dependencies{
		readBootstrapToken: func(io.Writer) (string, error) { read = true; return "", nil },
		runEnrollment: func(_ context.Context, _ string, origin, token string, resume, reEnroll bool) (enrollment.EnrollmentResult, error) {
			if !resume || reEnroll || origin != "" || token != "" {
				t.Fatalf("resume args = origin %q token %q resume %v reenroll %v", origin, token, resume, reEnroll)
			}
			return enrollment.EnrollmentResult{}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), []string{"--resume"}, &stdout, &stderr, deps)
	if code != 0 || read {
		t.Fatalf("runEnrollment() = %d, prompt called=%v stderr=%q", code, read, stderr.String())
	}
}

func TestRunEnrollmentReEnrollmentRequiresNewTokenAndExplicitFlag(t *testing.T) {
	read := false
	called := false
	deps := dependencies{
		readBootstrapToken: func(io.Writer) (string, error) { read = true; return "fresh-token", nil },
		runEnrollment: func(_ context.Context, _ string, origin, token string, resume, reEnroll bool) (enrollment.EnrollmentResult, error) {
			called = true
			if !reEnroll || resume || token != "fresh-token" || origin != "https://core.example" {
				t.Fatalf("reenrollment args origin=%q token=%q resume=%v reenroll=%v", origin, token, resume, reEnroll)
			}
			return enrollment.EnrollmentResult{}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), []string{"--re-enroll", "--core-origin", "https://core.example"}, &stdout, &stderr, deps)
	if code != 0 || !called || !read {
		t.Fatalf("runEnrollment()=%d called=%v prompted=%v stderr=%q", code, called, read, stderr.String())
	}
}

func TestRunEnrollmentRequiresCoreOriginBeforePrompt(t *testing.T) {
	read := false
	deps := dependencies{readBootstrapToken: func(io.Writer) (string, error) { read = true; return "secret", nil }}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), nil, &stdout, &stderr, deps)
	if code != 2 || read {
		t.Fatalf("runEnrollment() = %d, prompt called=%v", code, read)
	}
}

func TestRunEnrollmentFleetUsesHiddenInvitationAndExplicitProvisioning(t *testing.T) {
	var gotState, gotOrigin, gotInvitation, provisionedEndpoint string
	prompted := false
	deps := dependencies{
		readFleetInvitationToken: func(_ io.Writer) (string, error) { prompted = true; return "invitation-secret", nil },
		runFleetEnrollment: func(_ context.Context, statePath, origin, invitation string, resume bool) (fleetdomain.Result, error) {
			gotState, gotOrigin, gotInvitation = statePath, origin, invitation
			if resume {
				t.Fatal("unexpected Fleet resume mode")
			}
			return fleetdomain.Result{AgentID: "agent-id", NodeID: "node-id", CredentialID: "credential-id", CredentialScope: "opamp.connect", OpAMPEndpoint: "wss://fleet.example/opamp"}, nil
		},
		provisionFleetSupervisor: func(_ string, endpoint string) error { provisionedEndpoint = endpoint; return nil },
	}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), []string{"--fleet-origin", "https://fleet.example", "--state-path", "state.db", "--provision-supervisor"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("runEnrollment() = %d stderr=%q", code, stderr.String())
	}
	if !prompted || gotState != "state.db" || gotOrigin != "https://fleet.example" || gotInvitation != "invitation-secret" {
		t.Fatalf("Fleet enrollment args prompted=%v state=%q origin=%q invitation=%q", prompted, gotState, gotOrigin, gotInvitation)
	}
	if provisionedEndpoint != "wss://fleet.example/opamp" {
		t.Fatalf("provisioned endpoint=%q", provisionedEndpoint)
	}
	if strings.Contains(stdout.String()+stderr.String(), "invitation-secret") || strings.Contains(stdout.String()+stderr.String(), "fleet-secret") {
		t.Fatal("Fleet CLI printed enrollment secret material")
	}
}

func TestRunEnrollmentFleetResumeDoesNotPromptForInvitation(t *testing.T) {
	prompted := false
	called := false
	deps := dependencies{
		readFleetInvitationToken: func(io.Writer) (string, error) { prompted = true; return "secret", nil },
		runFleetEnrollment: func(_ context.Context, _ string, origin, invitation string, resume bool) (fleetdomain.Result, error) {
			called = true
			if !resume || invitation != "" || origin != "https://fleet.example" {
				t.Fatalf("Fleet resume args origin=%q invitation=%q resume=%v", origin, invitation, resume)
			}
			return fleetdomain.Result{AgentID: "agent-id", NodeID: "node-id", OpAMPEndpoint: "wss://fleet.example/opamp"}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), []string{"--resume", "--fleet-origin", "https://fleet.example"}, &stdout, &stderr, deps)
	if code != 0 || !called || prompted {
		t.Fatalf("runEnrollment()=%d called=%v prompted=%v stderr=%q", code, called, prompted, stderr.String())
	}
}

package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

func TestRunEnrollmentUsesHiddenTokenProviderAndDoesNotPrintCredentials(t *testing.T) {
	var gotState, gotOrigin, gotToken string
	deps := dependencies{
		readBootstrapToken: func(_ io.Writer) (string, error) { return "bootstrap-secret", nil },
		runEnrollment: func(_ context.Context, statePath, coreOrigin, token string, resume bool) (enrollment.EnrollmentResult, error) {
			gotState, gotOrigin, gotToken = statePath, coreOrigin, token
			if resume {
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
		runEnrollment: func(_ context.Context, _ string, origin, token string, resume bool) (enrollment.EnrollmentResult, error) {
			if !resume || origin != "" || token != "" {
				t.Fatalf("resume args = origin %q token %q resume %v", origin, token, resume)
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

func TestRunEnrollmentRequiresCoreOriginBeforePrompt(t *testing.T) {
	read := false
	deps := dependencies{readBootstrapToken: func(io.Writer) (string, error) { read = true; return "secret", nil }}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), nil, &stdout, &stderr, deps)
	if code != 2 || read {
		t.Fatalf("runEnrollment() = %d, prompt called=%v", code, read)
	}
}

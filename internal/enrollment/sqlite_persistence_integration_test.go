package enrollment_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/credentialstore"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestSQLiteAndCredentialStoreKeepEnrollmentSecretsSeparated(t *testing.T) {
	if !canPersistPlatformCredentialForTest() {
		t.Skip("the Windows CredentialStore integration requires an elevated process")
	}
	const (
		bootstrapToken = "arimms_bt_sqlite-persistence-test-secret"
		accessToken    = "arimms_ac_sqlite-persistence-test-secret"
		installationID = "10000000-0000-4000-8000-000000000002"
	)
	origin, err := enrollment.ParseCoreOrigin("https://core.example.test")
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "agent-state.db")
	parsedInstallationID, err := identity.ParseInstallationID(installationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentstate.NewIdentityStore(statePath).EnsureInstallationID(parsedInstallationID); err != nil {
		t.Fatalf("initialize installation identity: %v", err)
	}
	repository := agentstate.NewEnrollmentStore(statePath)
	credentials := credentialstore.New(statePath)
	service := enrollment.Service{
		Repository:  repository,
		Credentials: credentials,
		Client: staticEnrollmentClient{response: enrollment.EnrollmentResponse{
			EnrollmentID:   "10000000-0000-4000-8000-000000000003",
			InstallationID: installationID,
			Binding: enrollment.Binding{
				ID: "10000000-0000-4000-8000-000000000004", NodeID: "10000000-0000-4000-8000-000000000005", Status: "active",
			},
			Credential: enrollment.Credential{Type: "bearer", AccessToken: accessToken},
			ControlPlane: enrollment.ControlPlane{
				BaseURL: "https://core.example.test/api/v1/agent-control", PollIntervalSeconds: 30,
			},
		}},
	}
	result, err := service.Start(context.Background(), origin, enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: installationID},
		Agent:        enrollment.Agent{Version: "0.1.4"},
		Environment:  enrollment.Environment{OS: "linux", Architecture: "amd64"},
	}, bootstrapToken)
	if err != nil || result.Binding.Status != "active" {
		t.Fatalf("Start() result=%+v, err=%v", result, err)
	}
	attempt, err := repository.LoadAttempt()
	if err != nil || attempt.Status != enrollment.AttemptRegistered {
		t.Fatalf("registered attempt=%+v, err=%v", attempt, err)
	}
	storedAccessToken, err := credentials.Get(attempt.AccessCredentialRef)
	if err != nil || storedAccessToken != accessToken {
		t.Fatal("Agent credential was not retrievable from CredentialStore")
	}
	if _, err := credentials.Get(attempt.BootstrapCredentialRef); !errors.Is(err, enrollment.ErrCredentialNotFound) {
		t.Fatalf("bootstrap token cleanup error=%v", err)
	}
	files, err := filepath.Glob(statePath + "*")
	if err != nil {
		t.Fatal("find SQLite database files")
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read SQLite state: %v", err)
		}
		if bytes.Contains(content, []byte(bootstrapToken)) || bytes.Contains(content, []byte(accessToken)) {
			clear(content)
			t.Fatal("SQLite state contains plaintext enrollment secret")
		}
		clear(content)
	}
}

type staticEnrollmentClient struct {
	response enrollment.EnrollmentResponse
}

func (c staticEnrollmentClient) Enroll(context.Context, enrollment.CoreOrigin, enrollment.EnrollmentRequest) (enrollment.EnrollmentResponse, error) {
	return c.response, nil
}

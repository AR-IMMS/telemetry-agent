package agentstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

func TestSQLiteEnrollmentAttemptSurvivesRestartWithoutSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	identityID := mustInstallationID(t, "5baf0cba-cf5a-4d39-a3d4-e76263b70810")
	if _, err := NewIdentityStore(path).EnsureInstallationID(identityID); err != nil {
		t.Fatal(err)
	}
	store := NewEnrollmentStore(path)
	attempt := pendingEnrollmentAttempt(identityID.String(), "10000000-0000-4000-8000-000000000001")
	if err := store.CreateAttempt(attempt); err != nil {
		t.Fatalf("CreateAttempt() error = %v", err)
	}
	restarted := NewEnrollmentStore(path)
	got, err := restarted.LoadAttempt()
	if err != nil {
		t.Fatalf("LoadAttempt() error = %v", err)
	}
	if got.Payload.AttemptID != attempt.Payload.AttemptID || got.Payload.Environment != attempt.Payload.Environment {
		t.Fatalf("persisted attempt = %+v, want %+v", got, attempt)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "bootstrap-secret") || strings.Contains(string(content), "agent-secret") {
		t.Fatal("SQLite state contains enrollment credentials")
	}
}

func TestSQLiteEnrollmentAttemptChangesAreScopedAndConcurrentCreateIsSerialized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	identityID := mustInstallationID(t, "5baf0cba-cf5a-4d39-a3d4-e76263b70810")
	if _, err := NewIdentityStore(path).EnsureInstallationID(identityID); err != nil {
		t.Fatal(err)
	}
	state := State{DesiredGeneration: 4, Dependencies: map[string]DependencyState{
		"redis": {Enabled: true},
	}}
	if err := NewSQLiteStore(path).Save(state); err != nil {
		t.Fatal(err)
	}
	storeA, storeB := NewEnrollmentStore(path), NewEnrollmentStore(path)
	attempts := []enrollment.Attempt{
		pendingEnrollmentAttempt(identityID.String(), "10000000-0000-4000-8000-000000000006"),
		pendingEnrollmentAttempt(identityID.String(), "10000000-0000-4000-8000-000000000007"),
	}
	var wait sync.WaitGroup
	errorsFound := make(chan error, len(attempts))
	for index, store := range []enrollment.AttemptRepository{storeA, storeB} {
		wait.Add(1)
		go func(index int, store enrollment.AttemptRepository) {
			defer wait.Done()
			errorsFound <- store.CreateAttempt(attempts[index])
		}(index, store)
	}
	wait.Wait()
	close(errorsFound)
	created, rejected := 0, 0
	for err := range errorsFound {
		switch {
		case err == nil:
			created++
		case errors.Is(err, enrollment.ErrAttemptExists):
			rejected++
		default:
			t.Fatalf("CreateAttempt() error = %v", err)
		}
	}
	if created != 1 || rejected != 1 {
		t.Fatalf("successful creates=%d rejected creates=%d, want 1 each", created, rejected)
	}
	loaded, err := storeA.LoadAttempt()
	if err != nil {
		t.Fatal(err)
	}
	result := enrollment.EnrollmentResult{
		EnrollmentID: "10000000-0000-4000-8000-000000000003", InstallationID: identityID.String(),
		Binding:      enrollment.Binding{ID: "10000000-0000-4000-8000-000000000004", NodeID: "10000000-0000-4000-8000-000000000005", Status: "active"},
		ControlPlane: enrollment.ControlPlane{BaseURL: "https://core.example/api", PollIntervalSeconds: 30},
	}
	if err := storeA.CompleteAttempt(loaded.Payload.AttemptID, result, "credential-ref"); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	current, err := NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if current.DesiredGeneration != state.DesiredGeneration || !current.Dependencies["redis"].Enabled {
		t.Fatalf("enrollment changed existing operational state: %+v", current)
	}
	completed, err := storeB.LoadAttempt()
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != enrollment.AttemptRegistered || completed.AccessCredentialRef != "credential-ref" {
		t.Fatalf("completed enrollment = %+v", completed)
	}
}

func TestSQLiteEnrollmentReEnrollmentRetainsPriorBindingUntilReplacementCompletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	installationID := "5baf0cba-cf5a-4d39-a3d4-e76263b70810"
	parsedID := mustInstallationID(t, installationID)
	if _, err := NewIdentityStore(path).EnsureInstallationID(parsedID); err != nil {
		t.Fatal(err)
	}
	store := NewEnrollmentStore(path)
	previous := pendingEnrollmentAttempt(installationID, "10000000-0000-4000-8000-000000000008")
	if err := store.CreateAttempt(previous); err != nil {
		t.Fatal(err)
	}
	priorResult := enrollment.EnrollmentResult{EnrollmentID: "10000000-0000-4000-8000-000000000003", InstallationID: installationID, Binding: enrollment.Binding{ID: "10000000-0000-4000-8000-000000000004", NodeID: "10000000-0000-4000-8000-000000000005", Status: "active"}}
	if err := store.CompleteAttempt(previous.Payload.AttemptID, priorResult, "old-ref"); err != nil {
		t.Fatal(err)
	}
	next := pendingEnrollmentAttempt(installationID, "10000000-0000-4000-8000-000000000009")
	if err := store.ReplaceForReEnrollment(previous.Payload.AttemptID, next); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadAttempt()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PreviousResult == nil || loaded.PreviousResult.Binding.ID != priorResult.Binding.ID || loaded.PreviousAccessCredentialRef != "old-ref" {
		t.Fatalf("prior enrollment not retained: %+v", loaded)
	}
	if err := store.CompleteAttempt(next.Payload.AttemptID, priorResult, "new-ref"); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearPreviousCredential(next.Payload.AttemptID); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadAttempt()
	if err != nil || loaded.PreviousAccessCredentialRef != "" || loaded.AccessCredentialRef != "new-ref" {
		t.Fatalf("credential refs after cleanup: %+v err=%v", loaded, err)
	}
}

func pendingEnrollmentAttempt(installationID, attemptID string) enrollment.Attempt {
	return enrollment.Attempt{
		Payload: enrollment.EnrollmentPayload{
			AttemptID: attemptID, Installation: enrollment.Installation{ID: installationID},
			Agent:       enrollment.Agent{Version: "0.1.4"},
			Environment: enrollment.Environment{OS: "linux", Architecture: "amd64"},
		},
		CoreOrigin: "https://core.example", BootstrapCredentialRef: "bootstrap-ref",
		AccessCredentialRef: "agent-ref", Status: enrollment.AttemptPending,
	}
}

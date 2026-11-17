package sqlite_test

import (
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	enrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/sqlite"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestRepositorySharesOperationalDatabaseAndCompatibilityFacade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-state.db")
	store := sqlitefixture.NewSQLiteStore(path)
	installationID, err := installationid.ParseInstallationID("5baf0cba-cf5a-4d39-a3d4-e76263b70810")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlitefixture.NewIdentityStore(path).EnsureInstallationID(installationID); err != nil {
		t.Fatalf("initialize installation identity: %v", err)
	}
	locker := &sync.Mutex{}
	repository := enrollmentsqlite.New(store.OpenDatabase, locker)
	attemptID := "10000000-0000-4000-8000-000000000001"
	attempt := enrollment.Attempt{
		Payload: enrollment.EnrollmentPayload{
			AttemptID: attemptID, Installation: enrollment.Installation{ID: installationID.String()},
			Agent:       enrollment.Agent{Version: "0.1.4"},
			Environment: enrollment.Environment{OS: "linux", Architecture: "amd64"},
		},
		CoreOrigin: "https://core.example", BootstrapCredentialRef: "bootstrap-ref",
		AccessCredentialRef: "agent-ref", Status: enrollment.AttemptPending,
	}
	if err := repository.CreateAttempt(attempt); err != nil {
		t.Fatalf("persist pending attempt: %v", err)
	}

	restarted := enrollmentsqlite.New(store.OpenDatabase, locker)
	got, err := restarted.LoadAttempt()
	if err != nil {
		t.Fatalf("load pending attempt after reopening: %v", err)
	}
	if !reflect.DeepEqual(got, attempt) {
		t.Fatalf("reopened attempt = %+v, want %+v", got, attempt)
	}

	compatibilityRepository := sqlitefixture.NewEnrollmentStore(path)
	got, err = compatibilityRepository.LoadAttempt()
	if err != nil || !reflect.DeepEqual(got, attempt) {
		t.Fatalf("compatibility facade attempt = %+v, err=%v; want %+v", got, err, attempt)
	}
}

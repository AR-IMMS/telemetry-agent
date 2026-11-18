package main

import (
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	identityhost "github.com/ar-imms/telemetry-agent/internal/identity/host"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func TestAgentEnvironmentEvidenceUsesPersistedInstallationID(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agent.db")
	wantID := mustIdentityID(t, "11111111-2222-4333-8444-555555555555")
	committed, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(wantID)
	if err != nil {
		t.Fatalf("persist Agent installation ID: %v", err)
	}

	evidence := newAgentEnvironmentEvidence(statePath)
	if evidence == nil {
		t.Fatal("environment evidence is nil")
	}
	if evidence.AgentID != committed.String() {
		t.Fatalf("environment agent ID = %q; want persisted ID %q", evidence.AgentID, committed.String())
	}
	if !hasAgentIDEvidence(evidence.Evidence, identityhost.EvidenceOutcomeObserved) {
		t.Fatalf("environment evidence does not show persisted ID provenance: %+v", evidence.Evidence)
	}

	otherID := mustIdentityID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	after, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(otherID)
	if err != nil {
		t.Fatalf("read identity after evidence collection: %v", err)
	}
	if after != committed {
		t.Fatalf("evidence collection replaced persisted identity: got %s, want %s", after.String(), committed.String())
	}
}

func TestAgentEnvironmentEvidenceDoesNotCreateMissingInstallationID(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agent.db")
	if _, err := sqlitefixture.NewStore(statePath).Update(func(*agentstate.State) error { return nil }); err != nil {
		t.Fatalf("initialize Agent state: %v", err)
	}

	evidence := newAgentEnvironmentEvidence(statePath)
	if evidence == nil || evidence.AgentID != "" || !hasAgentIDEvidence(evidence.Evidence, identityhost.EvidenceOutcomeUnavailable) {
		t.Fatalf("missing installation ID was not reported as unavailable: %+v", evidence)
	}

	db, err := app.NewAgentSQLiteStore(statePath).OpenDatabase()
	if err != nil {
		t.Fatalf("open Agent database after evidence collection: %v", err)
	}
	defer db.Close()
	_, initialized, err := identitysqlite.ReadInstallationID(db)
	if err != nil {
		t.Fatalf("read installation ID after evidence collection: %v", err)
	}
	if initialized {
		t.Fatal("environment collection initialized an installation ID")
	}
}

func hasAgentIDEvidence(facts []identityhost.EnvironmentFact, outcome identityhost.EvidenceOutcome) bool {
	for _, fact := range facts {
		if fact.Source == "agent-installation-id" && fact.Outcome == outcome {
			return true
		}
	}
	return false
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	identityhost "github.com/ar-imms/telemetry-agent/internal/identity/host"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	collectordomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/domain"
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

func TestAgentEnvironmentEvidenceIncludesVerifiedCollectorVersion(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agent.db")
	installationID := mustIdentityID(t, "11111111-2222-4333-8444-555555555555")
	if _, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(installationID); err != nil {
		t.Fatalf("persist Agent installation ID: %v", err)
	}

	artifact, err := bootstrap.SelectArtifact(identityhost.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH})
	if err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(t.TempDir(), "collector")
	versionDir := filepath.Join(installDir, "versions", fmt.Sprintf("otelcol-contrib-%s-%s-%s", artifact.Version, artifact.OS, artifact.Architecture))
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(versionDir, artifact.BinaryName)
	binary := []byte("pinned Collector fixture")
	if err := os.WriteFile(binaryPath, binary, 0o600); err != nil {
		t.Fatal(err)
	}
	binaryDigest := sha256.Sum256(binary)
	metadata, err := json.Marshal(map[string]string{
		"version": artifact.Version, "os": artifact.OS, "architecture": artifact.Architecture,
		"archive_sha256": artifact.ArchiveSHA256, "binary_sha256": hex.EncodeToString(binaryDigest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "install.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	state := agentstate.State{Collector: collectordomain.CollectorContext{BinaryPath: binaryPath}}
	if err := app.NewAgentSQLiteStore(statePath).Save(state); err != nil {
		t.Fatalf("persist managed Collector path: %v", err)
	}

	evidence := newAgentEnvironmentEvidence(statePath)
	if evidence.CollectorVersion != artifact.Version {
		t.Fatalf("Collector version evidence = %q; want verified version %q", evidence.CollectorVersion, artifact.Version)
	}
	if !hasEnvironmentEvidence(evidence.Evidence, "collector-installed-version", identityhost.EvidenceOutcomeObserved) {
		t.Fatalf("verified Collector version has no provenance fact: %+v", evidence.Evidence)
	}
	if err := os.WriteFile(binaryPath, []byte("tampered Collector"), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence = newAgentEnvironmentEvidence(statePath)
	if evidence.CollectorVersion != "" || !hasEnvironmentEvidence(evidence.Evidence, "collector-installed-version", identityhost.EvidenceOutcomeUnavailable) {
		t.Fatalf("tampered Collector version was not reported as unavailable: %+v", evidence)
	}
}

func TestAgentEnvironmentEvidenceIncludesVerifiedSupervisorVersion(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agent.db")
	installationID := mustIdentityID(t, "11111111-2222-4333-8444-555555555555")
	if _, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(installationID); err != nil {
		t.Fatalf("persist Agent installation ID: %v", err)
	}
	const supervisorPath = `C:\ProgramData\AR-IMMS\versions\opampsupervisor.exe`
	state := agentstate.State{}
	state.Supervisor.BinaryPath = supervisorPath
	if err := app.NewAgentSQLiteStore(statePath).Save(state); err != nil {
		t.Fatalf("persist managed Supervisor path: %v", err)
	}

	var inspectedCollectorPath, inspectedSupervisorPath string
	evidence := newAgentEnvironmentEvidenceWithVersionInspector(statePath, func(collectorPath, supervisorPath string) identityhost.ManagedRuntimeVersions {
		inspectedCollectorPath = collectorPath
		inspectedSupervisorPath = supervisorPath
		return identityhost.ManagedRuntimeVersions{Supervisor: "0.160.0"}
	})
	if inspectedCollectorPath != "" || inspectedSupervisorPath != supervisorPath {
		t.Fatalf("runtime version inspector paths = (%q, %q); want empty Collector and Supervisor path %q", inspectedCollectorPath, inspectedSupervisorPath, supervisorPath)
	}
	if evidence.SupervisorVersion != "0.160.0" {
		t.Fatalf("Supervisor version evidence = %q; want verified version 0.160.0", evidence.SupervisorVersion)
	}
	if !hasEnvironmentEvidence(evidence.Evidence, "supervisor-installed-version", identityhost.EvidenceOutcomeObserved) {
		t.Fatalf("verified Supervisor version has no provenance fact: %+v", evidence.Evidence)
	}
}

func hasAgentIDEvidence(facts []identityhost.EnvironmentFact, outcome identityhost.EvidenceOutcome) bool {
	return hasEnvironmentEvidence(facts, "agent-installation-id", outcome)
}

func hasEnvironmentEvidence(facts []identityhost.EnvironmentFact, source string, outcome identityhost.EvidenceOutcome) bool {
	for _, fact := range facts {
		if fact.Source == source && fact.Outcome == outcome {
			return true
		}
	}
	return false
}

package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identityhost "github.com/ar-imms/telemetry-agent/internal/identity/host"
)

func TestReporterIncludesEnvironmentEvidenceInLocalSnapshot(t *testing.T) {
	observedAt := time.Date(2026, time.November, 18, 4, 0, 0, 0, time.UTC)
	evidence := &identityhost.EnvironmentEvidence{
		AgentID:           "11111111-2222-4333-8444-555555555555",
		OS:                "windows",
		Architecture:      "amd64",
		AgentVersion:      "v0.1.4a-next",
		CollectorVersion:  "0.160.0",
		SupervisorVersion: "0.160.0",
		HostID:            "opaque-host-id",
		HostIDSource:      "windows-machine-guid",
		HostType:          identityhost.HostTypeVM,
		ComputeType:       identityhost.ComputeTypeVM,
		Orchestrator:      identityhost.OrchestratorNone,
		DeploymentScope:   identityhost.DeploymentScopeHostService,
		ObservedAt:        observedAt,
		Evidence: []identityhost.EnvironmentFact{{
			Source: "windows-machine-guid", Outcome: identityhost.EvidenceOutcomeObserved,
			Confidence: identityhost.EvidenceConfidenceInferred, ObservedAt: observedAt,
		}},
	}

	reporter := NewReporterWithEnvironmentEvidence(RuntimeObservation{}, evidence)
	evidence.AgentID = "changed-after-construction"
	evidence.Evidence[0].Reason = "changed-after-construction"

	snapshot := reporter.SnapshotContext(context.Background())
	if snapshot.EnvironmentEvidence == nil {
		t.Fatal("health snapshot omitted environment evidence")
	}
	if snapshot.EnvironmentEvidence.AgentID != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("snapshot agent ID = %q; want persisted installation ID", snapshot.EnvironmentEvidence.AgentID)
	}
	if snapshot.EnvironmentEvidence.HostType != identityhost.HostTypeVM || snapshot.EnvironmentEvidence.HostIDSource != "windows-machine-guid" {
		t.Fatalf("snapshot lost classification provenance: %+v", snapshot.EnvironmentEvidence)
	}
	if snapshot.EnvironmentEvidence.CollectorVersion != "0.160.0" || snapshot.EnvironmentEvidence.SupervisorVersion != "0.160.0" {
		t.Fatalf("snapshot lost verified managed runtime versions: %+v", snapshot.EnvironmentEvidence)
	}
	if snapshot.EnvironmentEvidence.Evidence[0].Reason != "" {
		t.Fatalf("snapshot shares mutable evidence slice: %+v", snapshot.EnvironmentEvidence.Evidence[0])
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	NewHTTPHandler(reporter).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status response code = %d; want %d", recorder.Code, http.StatusOK)
	}
	var payload struct {
		EnvironmentEvidence *identityhost.EnvironmentEvidence `json:"environmentEvidence"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if payload.EnvironmentEvidence == nil || payload.EnvironmentEvidence.HostIDSource != "windows-machine-guid" || payload.EnvironmentEvidence.CollectorVersion != "0.160.0" || payload.EnvironmentEvidence.SupervisorVersion != "0.160.0" {
		t.Fatalf("status response environment evidence = %+v", payload.EnvironmentEvidence)
	}
}

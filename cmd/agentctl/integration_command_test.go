package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation"
)

func TestIntegrationStatusDoesNotInferHealthOrDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var out, stderr bytes.Buffer
	if code := runIntegration(context.Background(), []string{"status", "--state-path", path}, &out, &stderr); code != 0 {
		t.Fatalf("status exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "Convergence: Desired") || !strings.Contains(out.String(), "health and telemetry delivery are not inferred") {
		t.Fatalf("status output=%q", out.String())
	}
}

func TestIntegrationStatusDoesNotAcknowledgeStaleBaselineAsApplied(t *testing.T) {
	evidence := agentstate.ReconciliationEvidence{
		Target: agentstate.ReconciliationTarget{
			DesiredRevision:     12,
			DesiredGeneration:   4,
			BaselineFingerprint: "old-baseline",
		},
		Status: "Applied",
	}
	current := reconciliation.BaselineSnapshot{DesiredGeneration: 4, Fingerprint: "new-baseline"}
	if got := integrationConvergenceStatus(evidence, 12, current); got != "Pending" {
		t.Fatalf("status for old applied baseline = %q, want Pending", got)
	}
	evidence.Target.BaselineFingerprint = current.Fingerprint
	if got := integrationConvergenceStatus(evidence, 12, current); got != "Applied" {
		t.Fatalf("status for matching target = %q, want Applied", got)
	}
}

func TestIntegrationConfigureRejectsSecretSettingsWithoutEchoingThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var out, stderr bytes.Buffer
	code := runIntegration(context.Background(), []string{"configure", "--state-path", path, "--id", "redis-a", "--definition", "redis", "--version", "1.0.0", "--settings", `{"password":"super-secret-value"}`}, &out, &stderr)
	if code != 1 || strings.Contains(stderr.String(), "super-secret-value") {
		t.Fatalf("configure exit=%d stderr=%q", code, stderr.String())
	}
	snapshot, err := agentstate.NewSQLiteStore(path).IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 0 || snapshot.Revision != 0 {
		t.Fatalf("secret settings persisted: %+v", snapshot)
	}
}

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation"
	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
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
	evidence := reconciliationcontracts.TargetEvidence{
		Target: reconciliationcontracts.Target{
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
	snapshot, err := newTestDesiredStateRepository(sqlitefixture.NewSQLiteStore(path)).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 0 || snapshot.Revision != 0 {
		t.Fatalf("secret settings persisted: %+v", snapshot)
	}
}

func TestIntegrationDisablePreservesSettingsAndNoOpRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var stdout, stderr bytes.Buffer
	configure := []string{"configure", "--state-path", path, "--id", "redis-a", "--definition", "redis", "--version", "1.0.0", "--settings", `{"endpoint":"127.0.0.1:6379"}`}
	if code := runIntegration(context.Background(), configure, &stdout, &stderr); code != 0 {
		t.Fatalf("configure exit=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	disable := []string{"disable", "--state-path", path, "--id", "redis-a", "--expected-revision", "1"}
	if code := runIntegration(context.Background(), disable, &stdout, &stderr); code != 0 {
		t.Fatalf("disable exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Desired Integration disable: id=redis-a instance_revision=2 desired_set_revision=2") {
		t.Fatalf("disable output=%q", stdout.String())
	}
	snapshot, err := newTestDesiredStateRepository(sqlitefixture.NewSQLiteStore(path)).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 1 || snapshot.Instances[0].Enabled || snapshot.Instances[0].Configuration["endpoint"] != "127.0.0.1:6379" || snapshot.Instances[0].Revision != 2 || snapshot.Revision != 2 {
		t.Fatalf("after disable snapshot=%+v", snapshot)
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"disable", "--state-path", path, "--id", "redis-a", "--expected-revision", "2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("repeat disable exit=%d stderr=%q", code, stderr.String())
	}
	snapshot, err = newTestDesiredStateRepository(sqlitefixture.NewSQLiteStore(path)).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Instances[0].Revision != 2 || snapshot.Revision != 2 {
		t.Fatalf("repeat disable advanced revisions: %+v", snapshot)
	}
}

func TestIntegrationEnableMissingInstancePreservesNotFoundText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var stdout, stderr bytes.Buffer
	code := runIntegration(context.Background(), []string{"enable", "--state-path", path, "--id", "missing", "--expected-revision", "0"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "integration enable: sql: no rows in result set") {
		t.Fatalf("enable exit=%d stderr=%q", code, stderr.String())
	}
}

func TestIntegrationUpdateWithoutEnabledFlagPreservesStoredEnabledState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var stdout, stderr bytes.Buffer
	configure := []string{"configure", "--state-path", path, "--id", "redis-a", "--definition", "redis", "--version", "1.0.0", "--settings", `{"endpoint":"127.0.0.1:6379"}`}
	if code := runIntegration(context.Background(), configure, &stdout, &stderr); code != 0 {
		t.Fatalf("configure exit=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"disable", "--state-path", path, "--id", "redis-a", "--expected-revision", "1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("disable exit=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	update := []string{"update", "--state-path", path, "--id", "redis-a", "--definition", "redis", "--version", "1.0.0", "--expected-revision", "2", "--settings", `{"endpoint":"127.0.0.1:6380"}`}
	if code := runIntegration(context.Background(), update, &stdout, &stderr); code != 0 {
		t.Fatalf("update exit=%d stderr=%q", code, stderr.String())
	}
	snapshot, err := newTestDesiredStateRepository(sqlitefixture.NewSQLiteStore(path)).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 1 || snapshot.Instances[0].Enabled || snapshot.Instances[0].Configuration["endpoint"] != "127.0.0.1:6380" || snapshot.Instances[0].Revision != 3 {
		t.Fatalf("updated snapshot=%+v", snapshot)
	}
}

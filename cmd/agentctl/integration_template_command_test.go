package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

const redisTemplateJSON = `{"id":"redis-local","definition_id":"redis","definition_version":"1.0.0","status":"enabled","selector":{"resource_kinds":["container"],"exact_attributes":{"image":"redis"}},"bindings":[{"target":"endpoint","source":{"kind":"endpoint","endpoint_scope":"host","endpoint_protocol":"tcp","endpoint_provenance":"published_port"}}]}`

func TestIntegrationTemplateCommandsUseConfigurationWithoutReconciling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var stdout, stderr bytes.Buffer
	if code := runIntegration(context.Background(), []string{"template", "create", "--state-path", path, "--template-json", redisTemplateJSON, "--expected-revision", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "revision=1") || !strings.Contains(stdout.String(), "template_set_revision=1") {
		t.Fatalf("create output=%q", stdout.String())
	}
	desired, err := agentstate.NewSQLiteStore(path).IntegrationRepository().Snapshot()
	if err != nil || desired.Revision != 0 || len(desired.Instances) != 0 {
		t.Fatalf("template command mutated active desired state: %+v err=%v", desired, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runIntegration(context.Background(), []string{"template", "disable", "--state-path", path, "--id", "redis-local", "--expected-revision", "1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("disable exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "disabled") {
		t.Fatalf("disable output=%q", stdout.String())
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"template", "delete", "--state-path", path, "--id", "redis-local", "--expected-revision", "2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("delete exit=%d stderr=%s", code, stderr.String())
	}
	snapshot, err := agentstate.NewSQLiteStore(path).TemplateRepository().Snapshot()
	if err != nil || len(snapshot.Templates) != 1 || string(snapshot.Templates[0].Status) != "deleted" {
		t.Fatalf("deleted Template tombstone=%+v err=%v", snapshot, err)
	}
}

func TestIntegrationTemplateCommandRejectsCoreOwnerAndUnknownJSONFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	for _, raw := range []string{
		`{"id":"redis-local","definition_id":"redis","definition_version":"1.0.0","owner":"core","status":"enabled"}`,
		`{"id":"redis-local","definition_id":"redis","definition_version":"1.0.0","status":"enabled","surprise":true}`,
	} {
		var stdout, stderr bytes.Buffer
		code := runIntegration(context.Background(), []string{"template", "create", "--state-path", path, "--template-json", raw, "--expected-revision", "0"}, &stdout, &stderr)
		if code != 1 || strings.Contains(stderr.String(), `"core"`) {
			t.Fatalf("create raw=%s exit=%d stderr=%q", raw, code, stderr.String())
		}
	}
}

func TestIntegrationTemplateFileWorkflowExportsFullTemplateAndPreservesInlineInput(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	filePath := filepath.Join(t.TempDir(), "redis-monitoring.json")
	initial := `{"id":"redis-monitoring","definition_id":"redis","definition_version":"1.0.0","owner":"local","revision":1,"status":"enabled","selector":{"resource_kinds":["container"],"lifecycle_states":["running"],"exact_attributes":{"container.image.tag":"7.4.2-alpine"}},"settings":{"collection_interval":"15s"},"bindings":[{"target":"endpoint","source":{"kind":"endpoint","endpoint_scope":"host","endpoint_protocol":"tcp","endpoint_provenance":"published_port"}}]}`
	if err := os.WriteFile(filePath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runIntegration(context.Background(), []string{"template", "create", "--state-path", statePath, "--file", filePath, "--expected-revision", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create from file exit=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"template", "inspect", "--state-path", statePath, "--id", "redis-monitoring", "--output", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect JSON exit=%d stderr=%s", code, stderr.String())
	}
	var exported map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &exported); err != nil {
		t.Fatalf("inspect did not emit JSON: %v: %s", err, stdout.String())
	}
	if exported["id"] != "redis-monitoring" || exported["definition_version"] != "1.0.0" || exported["status"] != "enabled" {
		t.Fatalf("inspect omitted template identity: %#v", exported)
	}
	selector, ok := exported["selector"].(map[string]any)
	if !ok || selector["resource_kinds"] == nil || exported["settings"] == nil || exported["bindings"] == nil {
		t.Fatalf("inspect omitted selector, settings or bindings: %#v", exported)
	}
	updated := strings.Replace(initial, `"15s"`, `"30s"`, 1)
	if err := os.WriteFile(filePath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"template", "update", "--state-path", statePath, "--file", filePath, "--expected-revision", "1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("update from file exit=%d stderr=%s", code, stderr.String())
	}
	snapshot, err := agentstate.NewSQLiteStore(statePath).TemplateRepository().Snapshot()
	if err != nil || len(snapshot.Templates) != 1 || snapshot.Templates[0].Revision != 2 || snapshot.Templates[0].Settings["collection_interval"] != "30s" {
		t.Fatalf("updated template=%+v err=%v", snapshot, err)
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"template", "create", "--state-path", statePath, "--template-json", redisTemplateJSON, "--expected-revision", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("legacy inline JSON create exit=%d stderr=%s", code, stderr.String())
	}
}

func TestIntegrationTemplateFileInputRejectsInvalidJSONAndRevisionConflict(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	filePath := filepath.Join(t.TempDir(), "template.json")
	for _, raw := range []string{`{"id":`, `{"id":"x","surprise":true}`} {
		if err := os.WriteFile(filePath, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := runIntegration(context.Background(), []string{"template", "create", "--state-path", statePath, "--file", filePath, "--expected-revision", "0"}, &stdout, &stderr); code != 1 {
			t.Fatalf("invalid file JSON exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	}
	if err := os.WriteFile(filePath, []byte(redisTemplateJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runIntegration(context.Background(), []string{"template", "create", "--state-path", statePath, "--file", filePath, "--expected-revision", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create exit=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runIntegration(context.Background(), []string{"template", "update", "--state-path", statePath, "--file", filePath, "--expected-revision", "99"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "revision") {
		t.Fatalf("stale update exit=%d stderr=%q", code, stderr.String())
	}
	snapshot, err := agentstate.NewSQLiteStore(statePath).TemplateRepository().Snapshot()
	if err != nil || len(snapshot.Templates) != 1 || snapshot.Templates[0].Revision != 1 {
		t.Fatalf("stale update overwrote persisted Template: snapshot=%+v err=%v", snapshot, err)
	}
}

func TestIntegrationTemplateCommandRequiresExactlyOneInputSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var stdout, stderr bytes.Buffer
	code := runIntegration(context.Background(), []string{"template", "create", "--state-path", path, "--file", "template.json", "--template-json", redisTemplateJSON, "--expected-revision", "0"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "exactly one of --file or --template-json") {
		t.Fatalf("conflicting input flags exit=%d stderr=%q", code, stderr.String())
	}
}

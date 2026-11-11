package main

import (
	"bytes"
	"context"
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

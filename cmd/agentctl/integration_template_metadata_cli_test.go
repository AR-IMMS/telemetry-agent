package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationTemplateCLIExplainsInvalidSettingTypeAndMissingRequiredBinding(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	filePath := filepath.Join(t.TempDir(), "invalid-template.json")
	for _, test := range []struct {
		name, raw, want string
	}{
		{
			name: "wrong type",
			raw:  `{"id":"redis-invalid","definition_id":"redis","definition_version":"1.0.0","owner":"local","status":"enabled","settings":{"collection_interval":15},"bindings":[{"target":"endpoint","source":{"kind":"endpoint","endpoint_scope":"host","endpoint_protocol":"tcp","endpoint_provenance":"published_port"}}]}`,
			want: `setting "collection_interval" must be string`,
		},
		{
			name: "invalid duration",
			raw:  `{"id":"redis-invalid","definition_id":"redis","definition_version":"1.0.0","owner":"local","status":"enabled","settings":{"collection_interval":"0s"},"bindings":[{"target":"endpoint","source":{"kind":"endpoint","endpoint_scope":"host","endpoint_protocol":"tcp","endpoint_provenance":"published_port"}}]}`,
			want: `setting "collection_interval": must be a positive Go duration`,
		},
		{
			name: "invalid endpoint",
			raw:  `{"id":"redis-invalid","definition_id":"redis","definition_version":"1.0.0","owner":"local","status":"enabled","settings":{"endpoint":"redis://user:pass@host:6379"}}`,
			want: `setting "endpoint": must be host:port without URL or credential syntax`,
		},
		{
			name: "missing endpoint binding",
			raw:  `{"id":"redis-invalid","definition_id":"redis","definition_version":"1.0.0","owner":"local","status":"enabled"}`,
			want: `required Template setting "endpoint" needs a value or binding`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filePath, []byte(test.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := runIntegration(context.Background(), []string{"template", "create", "--state-path", statePath, "--file", filePath, "--expected-revision", "0"}, &stdout, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), test.want) || strings.Contains(stderr.String(), "user:pass") {
				t.Fatalf("invalid template exit=%d stderr=%q want=%q", code, stderr.String(), test.want)
			}
		})
	}
}

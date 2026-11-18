package dependency

import (
	"bytes"
	"context"
	"errors"
	"testing"

	runtimeDependencies "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

func TestListRendersAvailableDependencies(t *testing.T) {
	var stdout, stderr bytes.Buffer
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "retained")
	exitCode := List(ctx, &stdout, &stderr, func(gotContext context.Context) ([]runtimeDependencies.Definition, error) {
		if gotContext.Value(contextKey{}) != "retained" {
			t.Fatal("List() did not preserve the caller context")
		}
		return []runtimeDependencies.Definition{{
			Name: "node-exporter", DisplayName: "Node Exporter", Description: "Host metrics",
		}}, nil
	})

	if exitCode != 0 {
		t.Fatalf("List() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "Available dependencies:\n- node-exporter: Node Exporter — Host metrics\n"; got != want {
		t.Fatalf("List() stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("List() stderr = %q, want empty", stderr.String())
	}
}

func TestListReportsMissingCatalogReader(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := List(context.Background(), &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("List() exit code = %d, want 1", exitCode)
	}
	if got, want := stderr.String(), "list dependencies: lister is not configured\n"; got != want {
		t.Fatalf("List() stderr = %q, want %q", got, want)
	}
}

func TestListReportsEmptyCatalog(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := List(context.Background(), &stdout, &stderr, func(context.Context) ([]runtimeDependencies.Definition, error) {
		return nil, nil
	})

	if exitCode != 0 {
		t.Fatalf("List() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "No dependencies are available for this operating system.\n"; got != want {
		t.Fatalf("List() stdout = %q, want %q", got, want)
	}
}

func TestListReportsCatalogFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	wantErr := errors.New("catalog unavailable")
	exitCode := List(context.Background(), &stdout, &stderr, func(context.Context) ([]runtimeDependencies.Definition, error) {
		return nil, wantErr
	})

	if exitCode != 1 {
		t.Fatalf("List() exit code = %d, want 1", exitCode)
	}
	if got, want := stderr.String(), "list dependencies: catalog unavailable\n"; got != want {
		t.Fatalf("List() stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("List() stdout = %q, want empty", stdout.String())
	}
}

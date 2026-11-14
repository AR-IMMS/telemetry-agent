package main

import (
	"context"
	"runtime"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestDependencyOperationsInspectorReadsNativeManagedDependency(t *testing.T) {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	name := "node-exporter"
	if runtime.GOOS == "windows" {
		name = "windows-exporter"
	}
	service := dependency.Service{Catalog: catalog, OS: platform.OS}
	observation, err := (dependencyOperationsInspector{service: service}).Inspect(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect native managed dependency %q: %v", name, err)
	}
	if observation.Name != name || observation.Availability == "" || observation.Health == "" {
		t.Fatalf("native managed dependency observation=%+v", observation)
	}
	t.Logf("PR09_DEPENDENCY_INSPECTION platform=%s name=%s availability=%s health=%s drifted=%t",
		platform.OS, observation.Name, observation.Availability, observation.Health, observation.Drifted)
}

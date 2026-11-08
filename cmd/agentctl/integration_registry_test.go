package main

import (
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestProductionIntegrationRegistryFollowsVerifiedPlatformCatalog(t *testing.T) {
	for _, platform := range []identity.PlatformInfo{
		{OS: "windows", Architecture: "amd64"},
		{OS: "linux", Architecture: "amd64"},
	} {
		catalog, err := collectorruntime.StaticCatalog(platform)
		if err != nil {
			t.Fatal(err)
		}
		registry, err := productionIntegrationRegistry(catalog)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.ResolveDefinition("redis", "1.0.0"); err != nil {
			t.Fatalf("verified %s Redis definition not registered: %v", platform.OS, err)
		}
	}

	unverified, err := collectorruntime.NewCatalog("0.160.0", "empty-manifest", nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := productionIntegrationRegistry(unverified)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ResolveDefinition("redis", "1.0.0"); !errors.Is(err, integration.ErrUnknownDefinition) {
		t.Fatalf("unverified Redis definition lookup error = %v", err)
	}
}

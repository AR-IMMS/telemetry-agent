package main

import (
	"errors"
	"testing"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestResolveApprovalSettingsPreservesLegacyValidationErrors(t *testing.T) {
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolver := definitionapp.NewResolver(registry)
	instance := desiredstatedomain.Instance{
		ID: "approval-validation", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": ""}, Enabled: true, Source: desiredstatedomain.SourceDiscovery,
		Binding: &desiredstatedomain.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "redis-container"},
	}
	if _, err := resolveApprovalSettings(resolver, instance); !errors.Is(err, desiredstatedomain.ErrInvalidInstance) {
		t.Fatalf("invalid profile settings error = %v, want legacy invalid-instance classification", err)
	}

	instance.DefinitionID = "unknown-profile"
	instance.Configuration["endpoint"] = "127.0.0.1:6379"
	if _, err := resolveApprovalSettings(resolver, instance); !errors.Is(err, definitionsdomain.ErrUnknownDefinition) {
		t.Fatalf("unknown definition error = %v, want Monitoring's unknown-definition error", err)
	}
}

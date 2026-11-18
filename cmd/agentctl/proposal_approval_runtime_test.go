package main

import (
	"errors"
	"testing"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	proposalapp "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	configurationtemplatesdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
	monitoringtemplatesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
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
	if _, err := proposalapp.ResolveApprovalSettings(resolver, instance); !errors.Is(err, desiredstatedomain.ErrInvalidInstance) {
		t.Fatalf("invalid profile settings error = %v, want legacy invalid-instance classification", err)
	}

	instance.DefinitionID = "unknown-profile"
	instance.Configuration["endpoint"] = "127.0.0.1:6379"
	if _, err := proposalapp.ResolveApprovalSettings(resolver, instance); !errors.Is(err, definitionsdomain.ErrUnknownDefinition) {
		t.Fatalf("unknown definition error = %v, want Monitoring's unknown-definition error", err)
	}
}

func TestMonitoringTemplateInputsPreserveLifecycleAndBinding(t *testing.T) {
	template := configurationtemplatesdomain.Template{
		ID: "redis-template", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Owner: desiredstatedomain.TemplateOwnerLocal, Revision: 3, Status: configurationtemplatesdomain.StatusEnabled,
		Settings: map[string]any{"scrape_interval": "15s"},
		Bindings: []configurationtemplatesdomain.SettingBinding{{
			Target: "endpoint",
			Source: configurationtemplatesdomain.BindingSource{
				Kind: configurationtemplatesdomain.SourceEndpoint, EndpointScope: "host",
				EndpointProtocol: "tcp", EndpointProvenance: "published_port",
			},
		}},
	}
	inputs, err := proposalapp.MonitoringTemplateInputs([]configurationtemplatesdomain.Template{template})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 {
		t.Fatalf("Template inputs = %d, want 1", len(inputs))
	}
	input := inputs[0]
	if err := input.Validate(); err != nil {
		t.Fatalf("Monitoring input should preserve the valid Configuration Template: %v", err)
	}
	if input.ID != template.ID || input.Owner != string(template.Owner) || input.Revision != template.Revision ||
		!input.Evaluate || input.DefinitionID != template.DefinitionID || input.DefinitionVersion != template.DefinitionVersion ||
		input.Settings["scrape_interval"] != "15s" || len(input.Bindings) != 1 ||
		input.Bindings[0].Source.Kind != monitoringtemplatesdomain.TemplateSourceEndpoint ||
		input.Bindings[0].Source.EndpointProvenance != "published_port" {
		t.Fatalf("Monitoring Template input lost Configuration-owned fields: %+v", input)
	}

	template.Status = configurationtemplatesdomain.StatusDisabled
	inputs, err = proposalapp.MonitoringTemplateInputs([]configurationtemplatesdomain.Template{template})
	if err != nil {
		t.Fatal(err)
	}
	if inputs[0].Evaluate {
		t.Fatal("disabled Configuration Template must not be evaluated")
	}
}

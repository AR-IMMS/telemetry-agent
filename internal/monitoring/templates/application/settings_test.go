package application

import (
	"errors"
	"reflect"
	"testing"

	configurationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
)

func TestResolveSettingsPreservesDefaultAndBindingPrecedence(t *testing.T) {
	definition := settingsDefinition()
	template := settingsTemplate()
	template.Settings["collection_interval"] = "15s"
	resource := matchingdomain.MatchResource{
		Provider: "docker", ProviderInstance: "local", ResourceID: "container-abc",
		Attributes: map[string]string{"service.interval": "30s"},
		Endpoints:  []matchingdomain.MatchEndpoint{{Address: "127.0.0.1", Port: 6379, Scope: "host", Protocol: "tcp", Provenance: "published_port"}},
	}

	settings, err := ResolveSettings(template, definition, resource)
	if err != nil {
		t.Fatalf("ResolveSettings() error = %v", err)
	}
	want := map[string]any{"collection_interval": "15s", "endpoint": "127.0.0.1:6379"}
	if !reflect.DeepEqual(settings, want) {
		t.Fatalf("settings = %#v, want %#v", settings, want)
	}

	template.Settings["endpoint"] = "redis.internal:6379"
	if _, err := ResolveSettings(template, definition, resource); !errors.Is(err, ErrTemplateSettingsConflict) {
		t.Fatalf("fixed Template setting conflict = %v, want %v", err, ErrTemplateSettingsConflict)
	}

	definition.TemplateSettingRules["endpoint"] = definitions.TemplateSettingRule{
		Type: "string", Description: "Target endpoint", Required: true,
		AllowResourceBinding: true, BindingOverridesTemplate: true,
	}
	settings, err = ResolveSettings(template, definition, resource)
	if err != nil || settings["endpoint"] != "127.0.0.1:6379" {
		t.Fatalf("binding precedence settings=%#v error=%v", settings, err)
	}
}

func TestResolveSettingsReportsUnavailableBindingEvidence(t *testing.T) {
	_, err := ResolveSettings(settingsTemplate(), settingsDefinition(), matchingdomain.MatchResource{
		Provider: "docker", ProviderInstance: "local", ResourceID: "container-abc",
	})
	if !errors.Is(err, ErrTemplateValueUnavailable) {
		t.Fatalf("ResolveSettings() error = %v, want %v", err, ErrTemplateValueUnavailable)
	}
}

func settingsTemplate() templatedomain.Template {
	return templatedomain.Template{
		ID: "redis-default", Owner: configurationdomain.TemplateOwnerLocal, Revision: 1,
		Status: templatedomain.StatusEnabled, DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Settings: map[string]any{},
		Bindings: []templatedomain.SettingBinding{{Target: "endpoint", Source: templatedomain.BindingSource{
			Kind: templatedomain.SourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}

func settingsDefinition() definitions.Definition {
	return definitions.Definition{
		ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil },
		Contribute: func(map[string]any) (definitions.Contribution, error) { return definitions.NewContribution(nil), nil },
		TemplateSettingRules: map[string]definitions.TemplateSettingRule{
			"endpoint":            {Type: "string", Description: "Target endpoint", Required: true, AllowResourceBinding: true},
			"collection_interval": {Type: "string", Description: "Collection frequency", Default: "10s", DefaultOverridable: true},
		},
	}
}

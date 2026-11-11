package integration

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestValidateIntegrationTemplateRejectsMalformedOwnerAndSecrets(t *testing.T) {
	template := validTemplate()
	if err := ValidateIntegrationTemplate(template); err != nil {
		t.Fatalf("valid template error = %v", err)
	}
	for _, test := range []struct {
		name   string
		change func(*IntegrationTemplate)
	}{
		{"malformed ID", func(value *IntegrationTemplate) { value.ID = "Bad ID" }},
		{"zero revision", func(value *IntegrationTemplate) { value.Revision = 0 }},
		{"unknown owner", func(value *IntegrationTemplate) { value.Owner = "remote-ish" }},
		{"secret setting", func(value *IntegrationTemplate) { value.Settings["api_key"] = "plaintext" }},
		{"unsafe binding field", func(value *IntegrationTemplate) {
			value.Bindings[0].Source.Attribute = "container.environment.API_TOKEN"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneTemplateForTest(template)
			if candidate.Settings == nil {
				candidate.Settings = make(map[string]any)
			}
			test.change(&candidate)
			if err := ValidateIntegrationTemplate(candidate); !errors.Is(err, ErrInvalidTemplate) {
				t.Fatalf("ValidateIntegrationTemplate() error = %v, want %v", err, ErrInvalidTemplate)
			}
		})
	}
}

func TestGeneratedInstanceIDUsesStableLogicalIdentity(t *testing.T) {
	resource := ResourceIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-abc"}
	first, err := GeneratedInstanceID(TemplateOwnerLocal, "redis-default", resource, "redis")
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratedInstanceID(TemplateOwnerLocal, "redis-default", resource, "redis")
	if err != nil || first != second {
		t.Fatalf("same identity = %q, %q, error=%v", first, second, err)
	}
	for _, different := range []struct {
		owner       TemplateOwner
		templateID  string
		resource    ResourceIdentity
		integration string
	}{
		{TemplateOwnerCore, "redis-default", resource, "redis"},
		{TemplateOwnerLocal, "redis-other", resource, "redis"},
		{TemplateOwnerLocal, "redis-default", ResourceIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-def"}, "redis"},
		{TemplateOwnerLocal, "redis-default", resource, "postgres"},
	} {
		got, err := GeneratedInstanceID(different.owner, different.templateID, different.resource, different.integration)
		if err != nil || got == first {
			t.Fatalf("different logical identity = %q, want distinct ID; error=%v", got, err)
		}
	}
	if _, err := GeneratedInstanceID(TemplateOwnerLocal, "bad id", resource, "redis"); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("malformed identity error = %v", err)
	}
}

func TestTemplateInstanceRequiresCompleteProvenance(t *testing.T) {
	instance := IntegrationInstance{
		ID: "template-0123456789abcdef01234567", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Source: SourceTemplate,
		TemplateOwner: TemplateOwnerLocal, TemplateID: "redis-default", TemplateRevision: 2,
		Binding: &ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"},
	}
	if err := ValidateInstance(instance); err != nil {
		t.Fatalf("complete provenance error = %v", err)
	}
	instance.TemplateRevision = 0
	if err := ValidateInstance(instance); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("missing template revision error = %v, want %v", err, ErrInvalidInstance)
	}
}

func TestCloneIntegrationTemplateDoesNotShareSettingsOrSelectors(t *testing.T) {
	original := validTemplate()
	original.Settings["collection_interval"] = "10s"
	original.Selector = &MatchSelector{ResourceKinds: []string{"container"}, ExactAttributes: map[string]string{"container.image.repository": "redis"}}
	cloned, err := CloneIntegrationTemplate(original)
	if err != nil {
		t.Fatal(err)
	}
	original.Settings["collection_interval"] = "30s"
	original.Selector.ResourceKinds[0] = "service"
	if cloned.Settings["collection_interval"] != "10s" || cloned.Selector.ResourceKinds[0] != "container" {
		t.Fatalf("clone shared mutable data: settings=%#v selector=%#v", cloned.Settings, cloned.Selector)
	}
}

func TestRegistryPreservesTemplateSettingRulesInDefinitionCopies(t *testing.T) {
	definition := templateDefinition()
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.ResolveDefinition(definition.ID, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if rule := resolved.TemplateSettingRules["endpoint"]; !rule.AllowResourceBinding {
		t.Fatalf("endpoint rule lost during Registry copy: %+v", rule)
	}
	if rule := resolved.TemplateSettingRules["collection_interval"]; rule.Default != "10s" || !rule.DefaultOverridable {
		t.Fatalf("collection interval rule lost during Registry copy: %+v", rule)
	}
	resolved.TemplateSettingRules["endpoint"] = TemplateSettingRule{}
	again, err := registry.ResolveDefinition(definition.ID, definition.Version)
	if err != nil || !again.TemplateSettingRules["endpoint"].AllowResourceBinding {
		t.Fatalf("mutating returned definition changed Registry state: %+v error=%v", again.TemplateSettingRules, err)
	}
}

func TestResolveTemplateSettingsAppliesExplicitDefaultAndBindingPrecedence(t *testing.T) {
	definition := templateDefinition()
	template := validTemplate()
	template.Settings["collection_interval"] = "15s"
	resource := MatchResource{
		Provider: "docker", ProviderInstance: "local", ResourceID: "container-abc",
		Attributes: map[string]string{"service.interval": "30s"},
		Endpoints:  []MatchEndpoint{{Address: "127.0.0.1", Port: 6379, Scope: "host", Protocol: "tcp", Provenance: "published_port"}},
	}
	settings, err := ResolveTemplateSettings(template, definition, resource)
	if err != nil {
		t.Fatalf("ResolveTemplateSettings() error = %v", err)
	}
	want := map[string]any{"collection_interval": "15s", "endpoint": "127.0.0.1:6379"}
	if !reflect.DeepEqual(settings, want) {
		t.Fatalf("settings = %#v, want %#v", settings, want)
	}

	template.Settings["endpoint"] = "redis.internal:6379"
	if _, err := ResolveTemplateSettings(template, definition, resource); !errors.Is(err, ErrTemplateSettingsConflict) {
		t.Fatalf("fixed Template setting binding conflict = %v, want %v", err, ErrTemplateSettingsConflict)
	}

	definition.TemplateSettingRules["endpoint"] = TemplateSettingRule{
		AllowResourceBinding: true, BindingOverridesTemplate: true,
	}
	settings, err = ResolveTemplateSettings(template, definition, resource)
	if err != nil || settings["endpoint"] != "127.0.0.1:6379" {
		t.Fatalf("declared binding precedence settings=%#v error=%v", settings, err)
	}
}

func validTemplate() IntegrationTemplate {
	return IntegrationTemplate{
		ID: "redis-default", Owner: TemplateOwnerLocal, Revision: 1,
		Status: TemplateEnabled, DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Settings: map[string]any{},
		Bindings: []TemplateSettingBinding{{Target: "endpoint", Source: TemplateBindingSource{
			Kind: TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}

func templateDefinition() IntegrationDefinition {
	return IntegrationDefinition{
		ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil },
		Contribute: func(IntegrationInstance) (Contribution, error) { return Contribution{}, nil },
		TemplateSettingRules: map[string]TemplateSettingRule{
			"endpoint":            {AllowResourceBinding: true},
			"collection_interval": {Default: "10s", DefaultOverridable: true},
		},
	}
}

func cloneTemplateForTest(value IntegrationTemplate) IntegrationTemplate {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var copy IntegrationTemplate
	if err := json.Unmarshal(encoded, &copy); err != nil {
		panic(err)
	}
	return copy
}

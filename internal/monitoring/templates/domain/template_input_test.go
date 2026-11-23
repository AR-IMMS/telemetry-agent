package domain

import "testing"

func TestTemplateInputValidateAcceptsBoundedEndpointProjection(t *testing.T) {
	input := TemplateInput{
		ID: "redis-default", Owner: "local", Revision: 4, Evaluate: true,
		DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Settings: map[string]any{"collection_interval": "15s"},
		Bindings: []TemplateSettingBinding{{
			Target: "endpoint",
			Source: TemplateBindingSource{
				Kind: TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
			},
		}},
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestTemplateInputValidateRejectsDuplicateOrUnsafeBindings(t *testing.T) {
	base := TemplateInput{
		ID: "redis-default", Owner: "local", Revision: 1,
		DefinitionID: "redis", DefinitionVersion: "1.0.0", Settings: map[string]any{},
	}
	t.Run("duplicate target", func(t *testing.T) {
		input := base
		input.Bindings = []TemplateSettingBinding{
			{Target: "endpoint", Source: TemplateBindingSource{Kind: TemplateSourceAttribute, Attribute: "service.endpoint"}},
			{Target: "endpoint", Source: TemplateBindingSource{Kind: TemplateSourceAttribute, Attribute: "service.address"}},
		}
		if err := input.Validate(); err == nil {
			t.Fatal("Validate() accepted duplicate binding targets")
		}
	})
	t.Run("sensitive attribute", func(t *testing.T) {
		input := base
		input.Bindings = []TemplateSettingBinding{{
			Target: "endpoint", Source: TemplateBindingSource{Kind: TemplateSourceAttribute, Attribute: "container.env.password"},
		}}
		if err := input.Validate(); err == nil {
			t.Fatal("Validate() accepted a sensitive resource attribute")
		}
	})
	t.Run("invalid endpoint source", func(t *testing.T) {
		input := base
		input.Bindings = []TemplateSettingBinding{{
			Target: "endpoint", Source: TemplateBindingSource{
				Kind: TemplateSourceEndpoint, EndpointScope: "any", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
			},
		}}
		if err := input.Validate(); err == nil {
			t.Fatal("Validate() accepted an unsupported endpoint scope")
		}
	})
}

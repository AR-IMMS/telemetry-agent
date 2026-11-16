package domain

import (
	"encoding/json"
	"errors"
	"testing"

	configurationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
)

func TestTemplateJSONKeepsPersistedFieldNames(t *testing.T) {
	template := Template{
		ID: "redis-default", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Owner: configurationdomain.TemplateOwnerLocal, Revision: 1, Status: StatusEnabled,
		Settings: map[string]any{"endpoint": "127.0.0.1:6379"},
		Bindings: []SettingBinding{{Target: "endpoint", Source: BindingSource{
			Kind: SourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
	encoded, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"id":"redis-default","definition_id":"redis","definition_version":"1.0.0","owner":"local","revision":1,"status":"enabled","settings":{"endpoint":"127.0.0.1:6379"},"bindings":[{"target":"endpoint","source":{"kind":"endpoint","endpoint_scope":"host","endpoint_protocol":"tcp","endpoint_provenance":"published_port"}}]}`
	if string(encoded) != want {
		t.Fatalf("serialized Template = %s, want %s", encoded, want)
	}
}

func TestTemplateValidationRejectsSensitiveOrMalformedBindings(t *testing.T) {
	valid := templateFixture()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Template rejected: %v", err)
	}

	secret := templateFixture()
	secret.Settings["api_key"] = "must-not-persist"
	if err := secret.Validate(); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("secret setting error = %v, want %v", err, ErrInvalidTemplate)
	}

	unsafeBinding := templateFixture()
	unsafeBinding.Bindings[0].Source.Attribute = "container.environment.API_TOKEN"
	unsafeBinding.Bindings[0].Source.Kind = SourceAttribute
	if err := unsafeBinding.Validate(); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("unsafe binding error = %v, want %v", err, ErrInvalidTemplate)
	}
}

func TestCloneTemplateCopiesSettingsAndSelector(t *testing.T) {
	original := templateFixture()
	original.Selector = &matchingdomain.MatchSelector{
		ResourceKinds:   []string{"container"},
		ExactAttributes: map[string]string{"container.image.repository": "redis"},
	}
	clone, err := CloneTemplate(original)
	if err != nil {
		t.Fatal(err)
	}
	original.Settings["collection_interval"] = "30s"
	original.Selector.ResourceKinds[0] = "service"
	if clone.Settings["collection_interval"] != "10s" || clone.Selector.ResourceKinds[0] != "container" {
		t.Fatalf("clone shares mutable state: settings=%#v selector=%#v", clone.Settings, clone.Selector)
	}
}

func templateFixture() Template {
	return Template{
		ID: "redis-default", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Owner: configurationdomain.TemplateOwnerLocal, Revision: 1, Status: StatusEnabled,
		Settings: map[string]any{"collection_interval": "10s"},
		Bindings: []SettingBinding{{Target: "endpoint", Source: BindingSource{
			Kind: SourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}

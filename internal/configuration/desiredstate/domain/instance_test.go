package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestInstanceJSONPreservesPersistedShape(t *testing.T) {
	instance := Instance{
		ID: "redis-main", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
		Source:        SourceTemplate,
		Binding:       &ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-123"},
		TemplateOwner: TemplateOwnerLocal, TemplateID: "redis-default", TemplateRevision: 4,
		Enabled: true, Revision: 7,
	}
	encoded, err := json.Marshal(instance)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ID", "DefinitionID", "DefinitionVersion", "Configuration", "source", "resource_binding", "template_owner", "template_id", "template_revision", "Enabled", "Revision"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("persisted JSON field %q is missing: %s", key, encoded)
		}
	}
	var binding map[string]json.RawMessage
	if err := json.Unmarshal(fields["resource_binding"], &binding); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"provider", "provider_instance", "resource_id"} {
		if _, ok := binding[key]; !ok {
			t.Errorf("persisted resource binding field %q is missing: %s", key, fields["resource_binding"])
		}
	}
	for _, key := range []string{"id", "definition_id", "definition_version", "configuration", "binding", "enabled", "revision"} {
		if _, ok := fields[key]; ok {
			t.Errorf("unexpected renamed persisted JSON field %q: %s", key, encoded)
		}
	}
}

func TestValidateInstanceRequiresDiscoveryBinding(t *testing.T) {
	instance := validInstance()
	instance.Source = SourceDiscovery
	if err := instance.Validate(); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("Validate() error = %v, want ErrInvalidInstance", err)
	}
}

func TestValidateInstanceRejectsSecretBearingConfiguration(t *testing.T) {
	instance := validInstance()
	instance.Configuration = map[string]any{"api_token": "must-not-persist"}
	if err := instance.Validate(); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("Validate() error = %v, want ErrInvalidInstance", err)
	}
}

func validInstance() Instance {
	return Instance{
		ID: "redis-main", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
		Source:        SourceLocal,
	}
}

package domain

import (
	"testing"

	configurationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
)

func TestGeneratedInstanceIDPreservesPersistedIdentity(t *testing.T) {
	resource := configurationdomain.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-123"}
	got, err := GeneratedInstanceID(configurationdomain.TemplateOwnerLocal, "redis-main", resource, "redis")
	if err != nil {
		t.Fatal(err)
	}
	if got != "template-2ee50028fbc22648e20e9d07" {
		t.Fatalf("generated ID=%q, want persisted-compatible ID %q", got, "template-2ee50028fbc22648e20e9d07")
	}
}

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	operationsnats "github.com/ar-imms/telemetry-agent/internal/operations/nats"
)

func TestOperationsBindingLoaderTracksProtectedCredentialReplacementAndRevocation(t *testing.T) {
	credential := "first-protected-value"
	present := true
	base := operationsnats.Config{URL: "nats://broker.example:4222", Username: "agent-binding-a", CAFile: "ca.pem",
		InstallationID: "installation-1", BindingID: "binding-a", Stream: "REQUESTS", Durable: "AGENT_A"}
	loader := operationsBindingLoader(base, "revision-7", "ops-binding-a", func(reference string) (string, error) {
		if reference != "ops-binding-a" || !present {
			return "", enrollment.ErrCredentialNotFound
		}
		return credential, nil
	})
	first, err := loader(context.Background())
	if err != nil || !first.Active || first.Config.Password != credential || first.Revision != "revision-7" {
		t.Fatalf("initial binding=%+v err=%v", first, err)
	}
	credential = "rotated-protected-value"
	rotated, err := loader(context.Background())
	if err != nil || !rotated.Active || rotated.Config.Password != credential {
		t.Fatalf("rotated binding=%+v err=%v", rotated, err)
	}
	encoded, err := json.Marshal(rotated)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), credential) || strings.Contains(string(encoded), "password") {
		t.Fatalf("binding state serialization exposed credential material: %s", encoded)
	}
	present = false
	revoked, err := loader(context.Background())
	if err != nil || revoked.Active || revoked.Config.Password != "" || revoked.Revision != "revision-7" {
		t.Fatalf("revoked binding=%+v err=%v", revoked, err)
	}
}

func TestOperationsContainerRestartLocalOptInRequiresLiteralTrue(t *testing.T) {
	for _, value := range []string{"", "false", "1", "TRUE", " true"} {
		if operationsContainerRestartEnabled(value) {
			t.Errorf("local opt-in %q unexpectedly enabled container restart", value)
		}
	}
	if !operationsContainerRestartEnabled("true") {
		t.Fatal("literal true did not enable local container restart opt-in")
	}
}

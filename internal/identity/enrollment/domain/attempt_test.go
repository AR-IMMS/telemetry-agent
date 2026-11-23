package domain

import (
	"encoding/json"
	"testing"
)

func TestAttemptKeepsPersistedEnrollmentJSONShape(t *testing.T) {
	attempt := Attempt{
		Payload: EnrollmentPayload{
			AttemptID: "10000000-0000-4000-8000-000000000001",
			Installation: Installation{
				ID: "10000000-0000-4000-8000-000000000002",
			},
			Agent:       Agent{Version: "0.1.4"},
			Environment: Environment{OS: "windows", Architecture: "amd64"},
		},
		CoreOrigin:             "https://core.example.test",
		BootstrapCredentialRef: "bootstrap/attempt-1",
		AccessCredentialRef:    "agent/attempt-1",
		Status:                 AttemptPending,
	}

	encoded, err := json.Marshal(attempt)
	if err != nil {
		t.Fatalf("marshal persisted attempt: %v", err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatalf("decode persisted attempt shape: %v", err)
	}
	for _, key := range []string{
		"payload", "core_origin", "bootstrap_credential_ref", "access_credential_ref", "status",
	} {
		if _, ok := persisted[key]; !ok {
			t.Errorf("persisted attempt is missing key %q: %s", key, encoded)
		}
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(persisted["payload"], &payload); err != nil {
		t.Fatalf("decode persisted payload: %v", err)
	}
	if _, ok := payload["enrollment_attempt_id"]; !ok {
		t.Errorf("persisted payload lost enrollment_attempt_id: %s", persisted["payload"])
	}
}

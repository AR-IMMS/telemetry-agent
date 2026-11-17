package application

import "testing"

func TestEnrollmentResponseValidatesIdentityCredentialAndTrustedControlPlane(t *testing.T) {
	origin, err := ParseCoreOrigin("https://core.example.com")
	if err != nil {
		t.Fatal(err)
	}
	valid := EnrollmentResponse{
		EnrollmentID:   "10000000-0000-4000-8000-000000000001",
		InstallationID: "10000000-0000-4000-8000-000000000002",
		Binding: Binding{
			ID: "10000000-0000-4000-8000-000000000003", NodeID: "10000000-0000-4000-8000-000000000004", Status: "active",
		},
		Credential:   Credential{Type: "bearer", AccessToken: "secret"},
		ControlPlane: ControlPlane{BaseURL: "https://core.example.com/api/v1/agent-control", PollIntervalSeconds: 30},
	}
	const installationID = "10000000-0000-4000-8000-000000000002"
	if err := valid.Validate(installationID, origin); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*EnrollmentResponse)
	}{
		{name: "mismatched installation", mutate: func(value *EnrollmentResponse) { value.InstallationID = "10000000-0000-4000-8000-000000000005" }},
		{name: "invalid enrollment UUID", mutate: func(value *EnrollmentResponse) { value.EnrollmentID = "bad" }},
		{name: "invalid binding UUID", mutate: func(value *EnrollmentResponse) { value.Binding.ID = "bad" }},
		{name: "invalid Node UUID", mutate: func(value *EnrollmentResponse) { value.Binding.NodeID = "bad" }},
		{name: "inactive binding", mutate: func(value *EnrollmentResponse) { value.Binding.Status = "revoked" }},
		{name: "unsupported credential", mutate: func(value *EnrollmentResponse) { value.Credential.Type = "basic" }},
		{name: "empty credential", mutate: func(value *EnrollmentResponse) { value.Credential.AccessToken = "" }},
		{name: "untrusted control plane origin", mutate: func(value *EnrollmentResponse) { value.ControlPlane.BaseURL = "https://attacker.example/api" }},
		{name: "non-https control plane", mutate: func(value *EnrollmentResponse) { value.ControlPlane.BaseURL = "http://core.example.com/api" }},
		{name: "invalid poll interval", mutate: func(value *EnrollmentResponse) { value.ControlPlane.PollIntervalSeconds = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(installationID, origin); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}

package enrollment

import "testing"

func TestParseCoreOriginRequiresHTTPSOrigin(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "https origin", value: "https://core.example.com"},
		{name: "http rejected", value: "http://core.example.com", wantErr: true},
		{name: "credentials rejected", value: "https://user:pass@core.example.com", wantErr: true},
		{name: "path rejected", value: "https://core.example.com/api", wantErr: true},
		{name: "query rejected", value: "https://core.example.com?next=elsewhere", wantErr: true},
		{name: "host required", value: "https://", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseCoreOrigin(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseCoreOrigin(%q) error = %v, wantErr %v", test.value, err, test.wantErr)
			}
		})
	}
}

func TestValidateEnrollmentResponseRequiresMatchingInstallationAndTrustedControlPlane(t *testing.T) {
	origin, err := ParseCoreOrigin("https://core.example.com")
	if err != nil {
		t.Fatal(err)
	}
	response := EnrollmentResponse{
		EnrollmentID:   "enrollment-1",
		InstallationID: "installation-1",
		Binding: Binding{
			ID:     "binding-1",
			NodeID: "node-1",
			Status: "active",
		},
		Credential: Credential{Type: "bearer", AccessToken: "secret"},
		ControlPlane: ControlPlane{
			BaseURL:             "https://core.example.com/api/v1/agent-control",
			PollIntervalSeconds: 30,
		},
	}

	if err := response.Validate("installation-1", origin); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*EnrollmentResponse)
	}{
		{name: "mismatched installation", mutate: func(value *EnrollmentResponse) { value.InstallationID = "other" }},
		{name: "inactive binding", mutate: func(value *EnrollmentResponse) { value.Binding.Status = "revoked" }},
		{name: "unsupported credential", mutate: func(value *EnrollmentResponse) { value.Credential.Type = "basic" }},
		{name: "empty credential", mutate: func(value *EnrollmentResponse) { value.Credential.AccessToken = "" }},
		{name: "untrusted control plane origin", mutate: func(value *EnrollmentResponse) { value.ControlPlane.BaseURL = "https://attacker.example/api" }},
		{name: "non-https control plane", mutate: func(value *EnrollmentResponse) { value.ControlPlane.BaseURL = "http://core.example.com/api" }},
		{name: "invalid poll interval", mutate: func(value *EnrollmentResponse) { value.ControlPlane.PollIntervalSeconds = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := response
			test.mutate(&candidate)
			if err := candidate.Validate("installation-1", origin); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}

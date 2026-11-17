package domain

import "testing"

func TestEnrollmentPayloadValidateMatchesContractBounds(t *testing.T) {
	valid := EnrollmentPayload{
		AttemptID:    "10000000-0000-4000-8000-000000000001",
		Installation: Installation{ID: "10000000-0000-4000-8000-000000000002"},
		Agent:        Agent{Version: "0.1.4"},
		Environment:  Environment{OS: "linux", Architecture: "amd64", Hostname: "node-01"},
	}
	tests := []struct {
		name    string
		mutate  func(*EnrollmentPayload)
		wantErr bool
	}{
		{name: "valid"},
		{name: "invalid attempt UUID", mutate: func(p *EnrollmentPayload) { p.AttemptID = "attempt" }, wantErr: true},
		{name: "invalid installation UUID", mutate: func(p *EnrollmentPayload) { p.Installation.ID = "installation" }, wantErr: true},
		{name: "unsupported OS", mutate: func(p *EnrollmentPayload) { p.Environment.OS = "plan9" }, wantErr: true},
		{name: "unsupported architecture", mutate: func(p *EnrollmentPayload) { p.Environment.Architecture = "mips" }, wantErr: true},
		{name: "version too long", mutate: func(p *EnrollmentPayload) { p.Agent.Version = string(make([]byte, 65)) }, wantErr: true},
		{name: "empty provided hostname", mutate: func(p *EnrollmentPayload) { p.Environment.Hostname = " " }, wantErr: true},
		{name: "hostname too long", mutate: func(p *EnrollmentPayload) { p.Environment.Hostname = string(make([]byte, 256)) }, wantErr: true},
		{name: "metadata too large", mutate: func(p *EnrollmentPayload) { p.Metadata = map[string]any{"x": string(make([]byte, 4097))} }, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			err := candidate.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

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

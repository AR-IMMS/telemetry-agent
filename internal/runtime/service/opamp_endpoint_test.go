package agentinstallation

import "testing"

func TestValidateOpAMPServerURL(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "secure remote", value: "wss://opamp.example.test/v1/opamp", valid: true},
		{name: "loopback cleartext", value: "ws://127.0.0.1:4320/v1/opamp", valid: true},
		{name: "loopback IPv6", value: "ws://[::1]:4320/v1/opamp", valid: true},
		{name: "remote cleartext", value: "ws://opamp.example.test/v1/opamp"},
		{name: "http", value: "https://opamp.example.test/v1/opamp"},
		{name: "userinfo", value: "wss://user:pass@opamp.example.test/v1/opamp"},
		{name: "query", value: "wss://opamp.example.test/v1/opamp?token=secret"},
		{name: "invalid port", value: "wss://opamp.example.test:99999/v1/opamp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateOpAMPServerURL(test.value)
			if test.valid && err != nil {
				t.Fatalf("ValidateOpAMPServerURL(%q): %v", test.value, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("ValidateOpAMPServerURL(%q) unexpectedly succeeded", test.value)
			}
		})
	}
}

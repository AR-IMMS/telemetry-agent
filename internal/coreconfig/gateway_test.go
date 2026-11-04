package coreconfig

import (
	"math"
	"testing"
)

func TestValidateGatewaySnapshot(t *testing.T) {
	id := "11111111-2222-4333-8444-555555555555"
	tests := []struct {
		name     string
		snapshot GatewaySnapshot
		valid    bool
	}{
		{"DNS endpoint", GatewaySnapshot{id, 1, "gateway.example:4317"}, true},
		{"IPv6 endpoint", GatewaySnapshot{id, 1, "[::1]:4317"}, true},
		{"missing identity", GatewaySnapshot{"", 1, "gateway:4317"}, false},
		{"zero revision", GatewaySnapshot{id, 0, "gateway:4317"}, false},
		{"SQLite overflow", GatewaySnapshot{id, math.MaxUint64, "gateway:4317"}, false},
		{"URL syntax", GatewaySnapshot{id, 1, "https://gateway:4317"}, false},
		{"credentials", GatewaySnapshot{id, 1, "user@gateway:4317"}, false},
		{"whitespace", GatewaySnapshot{id, 1, "gateway:4317 "}, false},
		{"port overflow", GatewaySnapshot{id, 1, "gateway:65536"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSnapshot(test.snapshot)
			if (err == nil) != test.valid {
				t.Fatalf("ValidateSnapshot(%+v) error = %v, valid = %v", test.snapshot, err, test.valid)
			}
		})
	}
}

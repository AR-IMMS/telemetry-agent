package main

import "testing"

func TestLoadConfigRequiresEveryTLSAndAuthoritySetting(t *testing.T) {
	values := map[string]string{
		"OTLP_INGRESS_LISTEN":                 "0.0.0.0:4317",
		"OTLP_INGRESS_TLS_CERT_FILE":          "server.crt",
		"OTLP_INGRESS_TLS_KEY_FILE":           "server.key",
		"OTLP_INGRESS_FLEET_ORIGIN":           "https://fleet.example.test",
		"OTLP_INGRESS_FLEET_CA_FILE":          "fleet-ca.crt",
		"OTLP_INGRESS_FLEET_CLIENT_CERT_FILE": "ingress.crt",
		"OTLP_INGRESS_FLEET_CLIENT_KEY_FILE":  "ingress.key",
		"OTLP_INGRESS_GATEWAY_ORIGIN":         "http://otel-gateway:4318",
	}
	getenv := func(key string) string { return values[key] }
	if _, err := loadConfig(getenv); err != nil {
		t.Fatal(err)
	}
	for key := range values {
		value := values[key]
		values[key] = ""
		if _, err := loadConfig(getenv); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
		values[key] = value
	}
}

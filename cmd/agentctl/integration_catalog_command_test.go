package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestIntegrationCatalogListsRegisteredDefinitionsAndTemplateContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runIntegration(context.Background(), []string{"catalog", "list", "--output", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("catalog list exit=%d stderr=%s", code, stderr.String())
	}
	var result struct {
		Definitions []struct {
			ID           string `json:"id"`
			Version      string `json:"version"`
			Capabilities struct {
				Status string   `json:"status"`
				Needed []string `json:"required"`
			} `json:"capabilities"`
			Mappings []struct {
				ID         string `json:"id"`
				NativeName string `json:"native_name"`
				Kind       string `json:"kind"`
				Pipeline   string `json:"pipeline"`
			} `json:"capability_mappings"`
			TemplateSettings map[string]struct {
				Type                 string   `json:"type"`
				Required             bool     `json:"required"`
				HasDefault           bool     `json:"has_default"`
				Default              any      `json:"default"`
				DefaultOverridable   bool     `json:"default_overridable"`
				AllowResourceBinding bool     `json:"allow_resource_binding"`
				Constraints          []string `json:"constraints"`
			} `json:"template_settings"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("catalog output is not JSON: %v: %s", err, stdout.String())
	}
	if len(result.Definitions) != 1 || result.Definitions[0].ID != "redis" || result.Definitions[0].Version != "1.0.0" {
		t.Fatalf("catalog definitions=%+v", result.Definitions)
	}
	redis := result.Definitions[0]
	if len(redis.Capabilities.Needed) != 1 || redis.Capabilities.Needed[0] != "otel.receiver.redis.metrics" {
		t.Fatalf("Redis required capabilities=%v", redis.Capabilities.Needed)
	}
	if redis.Capabilities.Status != "supported" || len(redis.Mappings) != 1 || redis.Mappings[0].ID != "otel.receiver.redis.metrics" || redis.Mappings[0].NativeName != "redis" || redis.Mappings[0].Kind != "receiver" || redis.Mappings[0].Pipeline != "metrics" {
		t.Fatalf("Redis capability mapping=%+v status=%s", redis.Mappings, redis.Capabilities.Status)
	}
	interval := redis.TemplateSettings["collection_interval"]
	if interval.Type != "string" || !interval.HasDefault || interval.Default != "10s" || !interval.DefaultOverridable || len(interval.Constraints) == 0 {
		t.Fatalf("collection_interval metadata=%+v", interval)
	}
	endpoint := redis.TemplateSettings["endpoint"]
	if endpoint.Type != "string" || !endpoint.Required || !endpoint.AllowResourceBinding || len(endpoint.Constraints) == 0 {
		t.Fatalf("endpoint metadata=%+v", endpoint)
	}
	stdout.Reset()
	if code := runIntegration(context.Background(), []string{"catalog", "inspect", "--id", "redis", "--version", "1.0.0", "--output", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("catalog inspect JSON exit=%d stderr=%s", code, stderr.String())
	}
	var inspected integrationCatalogView
	if err := json.Unmarshal(stdout.Bytes(), &inspected); err != nil || len(inspected.Definitions) != 1 || inspected.Definitions[0].ID != "redis" {
		t.Fatalf("catalog inspect JSON=%s err=%v", stdout.String(), err)
	}
}

func TestIntegrationCatalogInspectRejectsUnknownDefinitionAndVersion(t *testing.T) {
	for _, args := range [][]string{
		{"catalog", "inspect", "--id", "grafana", "--version", "1.0.0"},
		{"catalog", "inspect", "--id", "redis", "--version", "9.9.9"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runIntegration(context.Background(), args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "unknown integration definition") || !strings.Contains(stderr.String(), "use catalog list or the Integration Developer Guide") {
			t.Fatalf("catalog inspect args=%v exit=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestIntegrationCatalogInspectTextExplainsSettingsAndNativeMapping(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runIntegration(context.Background(), []string{"catalog", "inspect", "--id", "redis", "--version", "1.0.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("catalog inspect exit=%d stderr=%s", code, stderr.String())
	}
	for _, expected := range []string{
		"host and port without URL or credential syntax",
		"port must be between 1 and 65535",
		"positive Go duration",
		"otel.receiver.redis.metrics -> receiver redis",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("catalog inspect omitted %q: %s", expected, stdout.String())
		}
	}
}

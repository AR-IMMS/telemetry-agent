package opampsupervisorconfig

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRenderAllowlistedFallbackAndAuthenticatedConfiguration(t *testing.T) {
	content, err := Render(Config{
		ServerEndpoint: "wss://opamp.example.test/v1/opamp",
		TokenFile:      "C:/ProgramData/AR-IMMS/credentials/opamp-supervisor.token",
		CollectorPath:  "C:/Program Files/AR-IMMS/collector/otelcol-contrib.exe",
		LocalConfig:    "C:/ProgramData/AR-IMMS/collector.yaml",
		FallbackConfig: "C:/ProgramData/AR-IMMS/collector.yaml",
		StateDirectory: "C:/ProgramData/AR-IMMS/opampsupervisor",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, expected := range []string{"auth: bearertokenauth", "accepts_remote_config: true", "reports_remote_config: true", "$REMOTE_CONFIG", "automatic_config_rollback: true", "filename:", "opamp-supervisor.token"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("Supervisor config lacks %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "access_token") || strings.Contains(text, "secret") {
		t.Fatalf("Supervisor config contains credential material: %s", text)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("rendered YAML is invalid: %v", err)
	}
}

func TestRenderRejectsMissingEndpointAndRequiredPaths(t *testing.T) {
	if _, err := Render(Config{}); err == nil {
		t.Fatal("empty Supervisor config was accepted")
	}
	base := Config{ServerEndpoint: "ws://127.0.0.1:4320/v1/opamp", CollectorPath: "collector", LocalConfig: "local.yaml", FallbackConfig: "fallback.yaml", StateDirectory: "state"}
	base.TokenFile = "token\nfile"
	if _, err := Render(base); err == nil {
		t.Fatal("token file containing a line break was accepted")
	}
}

func TestRenderUsesExplicitServerTrustRoot(t *testing.T) {
	content, err := Render(Config{
		ServerEndpoint: "wss://opamp.example.test/v1/opamp",
		ServerCAFile:   "C:/ProgramData/AR-IMMS/credentials/opamp-server-ca.pem",
		CollectorPath:  "collector",
		LocalConfig:    "local.yaml",
		FallbackConfig: "fallback.yaml",
		StateDirectory: "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	var rendered struct {
		Server struct {
			TLS struct {
				CAFile string `yaml:"ca_file"`
			} `yaml:"tls"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(content, &rendered); err != nil {
		t.Fatal(err)
	}
	if got := rendered.Server.TLS.CAFile; got != "C:/ProgramData/AR-IMMS/credentials/opamp-server-ca.pem" {
		t.Fatalf("Supervisor server TLS CA file = %q", got)
	}
}

func TestRenderRejectsInvalidServerTrustRootPath(t *testing.T) {
	base := Config{ServerEndpoint: "wss://opamp.example.test/v1/opamp", CollectorPath: "collector", LocalConfig: "local.yaml", FallbackConfig: "fallback.yaml", StateDirectory: "state"}
	for _, path := range []string{" \t", "ca.pem\nextra", "ca.pem\x00"} {
		base.ServerCAFile = path
		if _, err := Render(base); err == nil {
			t.Fatalf("CA path %q was accepted", path)
		}
	}
}

package opampsupervisorconfig

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config contains the small set of local fields the Agent owns for Supervisor.
type Config struct {
	ServerEndpoint string
	ServerCAFile   string
	TokenFile      string
	CollectorPath  string
	LocalConfig    string
	FallbackConfig string
	StateDirectory string
}

type supervisorConfig struct {
	Server       serverConfig     `yaml:"server"`
	Capabilities capabilityConfig `yaml:"capabilities"`
	Agent        agentConfig      `yaml:"agent"`
	Storage      storageConfig    `yaml:"storage"`
	Extensions   *extensionConfig `yaml:"extensions,omitempty"`
	Telemetry    telemetryConfig  `yaml:"telemetry"`
}

type serverConfig struct {
	Endpoint string           `yaml:"endpoint"`
	Auth     string           `yaml:"auth,omitempty"`
	TLS      *serverTLSConfig `yaml:"tls,omitempty"`
}

type serverTLSConfig struct {
	CAFile string `yaml:"ca_file"`
}

type capabilityConfig struct {
	AcceptsRemoteConfig bool `yaml:"accepts_remote_config"`
	ReportsRemoteConfig bool `yaml:"reports_remote_config"`
}

type agentConfig struct {
	Executable                  string   `yaml:"executable"`
	ConfigFiles                 []string `yaml:"config_files"`
	StartupFallbackConfigs      []string `yaml:"startup_fallback_configs"`
	AutomaticConfigRollback     bool     `yaml:"automatic_config_rollback"`
	CollectorCrashLogSnippetKiB int      `yaml:"collector_crash_log_snippet_kib"`
}

type storageConfig struct {
	Directory string `yaml:"directory"`
}

type extensionConfig struct {
	BearerTokenAuth bearerTokenAuth `yaml:"bearertokenauth"`
}

type bearerTokenAuth struct {
	Filename string `yaml:"filename"`
}

type telemetryConfig struct {
	Logs logConfig `yaml:"logs"`
}

type logConfig struct {
	Level string `yaml:"level"`
}

// Render validates local paths and emits deterministic, allowlisted YAML.
func Render(config Config) ([]byte, error) {
	if err := validate(config); err != nil {
		return nil, err
	}
	result := supervisorConfig{
		Server: serverConfig{Endpoint: config.ServerEndpoint},
		Capabilities: capabilityConfig{
			AcceptsRemoteConfig: true,
			ReportsRemoteConfig: true,
		},
		Agent: agentConfig{
			Executable:                  config.CollectorPath,
			ConfigFiles:                 []string{config.LocalConfig, "$REMOTE_CONFIG"},
			StartupFallbackConfigs:      []string{config.FallbackConfig},
			AutomaticConfigRollback:     true,
			CollectorCrashLogSnippetKiB: 4,
		},
		Storage:   storageConfig{Directory: config.StateDirectory},
		Telemetry: telemetryConfig{Logs: logConfig{Level: "info"}},
	}
	if config.ServerCAFile != "" {
		result.Server.TLS = &serverTLSConfig{CAFile: config.ServerCAFile}
	}
	if config.TokenFile != "" {
		result.Server.Auth = "bearertokenauth"
		result.Extensions = &extensionConfig{BearerTokenAuth: bearerTokenAuth{Filename: config.TokenFile}}
	}
	encoded, err := yaml.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode Supervisor configuration: %w", err)
	}
	return encoded, nil
}

func validate(config Config) error {
	endpoint, err := url.Parse(strings.TrimSpace(config.ServerEndpoint))
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (strings.ToLower(endpoint.Scheme) != "wss" && strings.ToLower(endpoint.Scheme) != "ws") {
		return fmt.Errorf("Supervisor OpAMP endpoint is invalid")
	}
	if strings.EqualFold(endpoint.Scheme, "ws") {
		host := strings.ToLower(endpoint.Hostname())
		address := net.ParseIP(host)
		if host != "localhost" && (address == nil || !address.IsLoopback()) {
			return fmt.Errorf("cleartext OpAMP is allowed only on loopback")
		}
	}
	for name, value := range map[string]string{
		"Collector executable":       config.CollectorPath,
		"local Collector config":     config.LocalConfig,
		"fallback Collector config":  config.FallbackConfig,
		"Supervisor state directory": config.StateDirectory,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s path is required", name)
		}
	}
	if strings.ContainsAny(config.TokenFile, "\r\n\x00") {
		return fmt.Errorf("Supervisor token-file path is invalid")
	}
	if config.ServerCAFile != "" && (strings.TrimSpace(config.ServerCAFile) == "" || strings.ContainsAny(config.ServerCAFile, "\r\n\x00")) {
		return fmt.Errorf("Supervisor server CA-file path is invalid")
	}
	return nil
}
